package raknet

import (
	"fmt"
	"net"
	"time"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/raknet/internal/message"
	"github.com/gamevidea/raknet/internal/protocol"
)

// Listener is an implementation of Raknet Listener built on top of a UDP socket. It provides an API
// to accept Raknet Connections and read and write MCPE game packets in an ordered and reliable way.
type Listener struct {
	guid   int64
	addr   *net.UDPAddr
	socket *net.UDPConn

	connections    map[string]*Connection
	blocked        map[string]uint64
	packetsPerSec  map[string]time.Time
	invalidPackets map[string]uint8

	buf    []byte
	buffer *buffer.Buffer
}

// Listen announces on the local network address. Creates a new Raknet Listener and binds the listener
// to the provided address. Returns an error if the address was invalid or in use already.
func Listen(addr string) (*Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	socket, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, protocol.MAX_MTU_SIZE)

	listener := &Listener{
		guid:           0,
		addr:           udpAddr,
		socket:         socket,
		connections:    map[string]*Connection{},
		blocked:        map[string]uint64{},
		packetsPerSec:  map[string]time.Time{},
		invalidPackets: map[string]uint8{},
		buf:            buf,
		buffer:         buffer.New(buf),
	}

	go listener.start()

	return listener, nil
}

// Returns the GUID of the listener.
func (l *Listener) Guid() int64 {
	return l.guid
}

// Returns the local address of the listener that the listener is bound to.
func (l *Listener) LocalAddr() *net.UDPAddr {
	return l.addr
}

// Starts listening for datagrams from the socket that the listener is bound to.
func (l *Listener) start() {
	for {
		l.buffer.Reset() // reset the buffer for next datagram

		_, addr, err := l.socket.ReadFromUDP(l.buf)
		if err != nil {
			fmt.Printf("Listener: %v\n", err)
			continue
		}

		if _, ok := l.connections[addr.String()]; ok {
			continue
		}

		if err := l.handle(addr); err != nil {
			fmt.Printf("Listener: %v\n", err)
			continue
		}
	}
}

// Handle is called when an incoming unconnected message is received on the socket. It handles the message
// by flushing the response for the message immediately.
func (l *Listener) handle(addr *net.UDPAddr) error {
	id, err := l.buffer.ReadUint8()
	if err != nil {
		return err
	}

	fmt.Printf("ID: %d\n", id)

	switch id {
	case message.IDUnconnectedPing, message.IDUnconnectedPingOpenConnections:
		msg := message.UnconnectedPing{}
		return l.handleUnconnectedPing(addr, msg)
	case message.IDOpenConnectionRequest1:
		msg := message.OpenConnectionRequest1{}
		return l.handleOpenConnectionRequest1(addr, msg)
	case message.IDOpenConnectionRequest2:
		msg := message.OpenConnectionRequest2{}
		return l.handleOpenConnectionRequest2(addr, msg)
	default:
		fmt.Printf("Listener: %v\n", err)
		return nil
	}
}

// Handles an incoming unconnected ping message
func (l *Listener) handleUnconnectedPing(addr *net.UDPAddr, msg message.UnconnectedPing) (err error) {
	if err = msg.Read(l.buffer); err != nil {
		return
	}

	resp := message.UnconnectedPong{
		SendTimestamp: msg.SendTimestamp,
		ServerGUID:    l.guid,
		Data:          []byte("MCPE;Dedicated Server;390;1.14.60;0;10;13253860892328930865;Bedrock level;Survival;1;19132;19133;"),
	}

	if err = resp.Write(l.buffer); err != nil {
		return
	}

	if _, err = l.socket.WriteTo(l.buffer.Bytes(), addr); err != nil {
		return
	}

	return
}

// Handles an open connection request 1 message
func (l *Listener) handleOpenConnectionRequest1(addr *net.UDPAddr, msg message.OpenConnectionRequest1) (err error) {
	if err = msg.Read(l.buffer); err != nil {
		return
	}

	if msg.Protocol != protocol.PROTOCOL_VERSION {
		resp := message.IncompatibleProtocolVersion{
			ServerProtocol: protocol.PROTOCOL_VERSION,
			ServerGUID:     l.guid,
		}

		if err = resp.Write(l.buffer); err != nil {
			return
		}

		_, err = l.socket.WriteTo(l.buffer.Bytes(), addr)
		return
	}

	mtu := uint16(msg.DiscoveringMTU)
	if mtu > protocol.MAX_MTU_SIZE || mtu < protocol.MIN_MTU_SIZE {
		mtu = protocol.MAX_MTU_SIZE
	}

	resp := message.OpenConnectionReply1{
		ServerGUID:             l.guid,
		Secure:                 false,
		ServerPreferredMTUSize: mtu,
	}

	if err = resp.Write(l.buffer); err != nil {
		return
	}

	if _, err = l.socket.WriteTo(l.buffer.Bytes(), addr); err != nil {
		return
	}

	return
}

// Handles an open connection request 2 message
func (l *Listener) handleOpenConnectionRequest2(addr *net.UDPAddr, msg message.OpenConnectionRequest2) (err error) {
	if err = msg.Read(l.buffer); err != nil {
		return
	}

	mtu := msg.ClientPreferredMTUSize
	if mtu > protocol.MAX_MTU_SIZE || mtu < protocol.MIN_MTU_SIZE {
		mtu = protocol.MAX_MTU_SIZE
	}

	resp := message.OpenConnectionReply2{
		ServerGUID:    l.guid,
		ClientAddress: *addr,
		MTUSize:       mtu,
		Secure:        false,
	}

	if err = resp.Write(l.buffer); err != nil {
		return
	}

	if _, err = l.socket.WriteTo(l.buffer.Bytes(), addr); err != nil {
		return
	}

	l.connections[addr.String()] = &Connection{}
	return
}
