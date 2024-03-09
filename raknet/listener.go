package raknet

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/raknet/internal/message"
	"github.com/gamevidea/raknet/internal/protocol"
)

// Wrapper carries the information about an outgoing raknet message such as the destination
// and the message itself. It is sent through a channel to the write loop that runs on a separate
// thread.
type Wrapper struct {
	dest net.UDPAddr
	msg  message.Message
}

// DatagramMetrics help in keeping record of the number of datagrams that we receive from a connection in a second
// It is useful for figuring out whether we are being spammed or flooded by large number of datagrams being
// sent by a socket address.
type DatagramMetrics struct {
	timestamp time.Time
	count     int
}

// Listener is an implementation of Raknet Listener built on top of a UDP socket. It provides an API
// to accept Raknet Connections and read and write MCPE game packets in an ordered and reliable way.
type Listener struct {
	addr   *net.UDPAddr
	socket *net.UDPConn
	guid   int64

	connections map[string]*Connection
	blocked     map[string]time.Time

	datagramMetrics   map[string]*DatagramMetrics
	datagramIntegrity map[string]byte

	reader *buffer.Buffer
	writer *buffer.Buffer
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

	listener := &Listener{
		addr:              udpAddr,
		socket:            socket,
		guid:              rand.Int63(),
		connections:       map[string]*Connection{},
		blocked:           map[string]time.Time{},
		datagramMetrics:   map[string]*DatagramMetrics{},
		datagramIntegrity: map[string]byte{},
		reader:            buffer.New(protocol.MAX_MTU_SIZE),
		writer:            buffer.New(protocol.MAX_MTU_SIZE),
	}

	ch := make(chan Wrapper)

	go listener.startReadLoop(ch)
	go listener.startWriteLoop(ch)

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

// Starts the read loop that continuously reads any datagrams from the udp socket.
func (l *Listener) startReadLoop(ch chan Wrapper) {
	for {
		_, addr, err := l.socket.ReadFromUDP(l.reader.Slice())
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		if _, ok := l.connections[addr.String()]; ok {
			continue
		}

		if err := l.handle(addr, ch); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		l.reader.Reset()
	}
}

// Starts the write loop that flushes the outgoing datagrams to the udp socket
func (l *Listener) startWriteLoop(ch chan Wrapper) {
	for {
		info := <-ch

		if err := info.msg.Write(l.writer); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		if _, err := l.socket.WriteTo(l.writer.Bytes(), &info.dest); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		l.writer.Reset()
	}
}

// Handle is called when an incoming unconnected message is received on the socket. It handles the message
// by flushing the response for the message immediately.
func (l *Listener) handle(addr *net.UDPAddr, ch chan Wrapper) error {
	id, err := l.reader.ReadUint8()
	if err != nil {
		return err
	}

	switch id {
	case message.IDUnconnectedPing, message.IDUnconnectedPingOpenConnections:
		return l.handleUnconnectedPing(addr, ch)
	case message.IDOpenConnectionRequest1:
		return l.handleOpenConnectionRequest1(addr, ch)
	case message.IDOpenConnectionRequest2:
		return l.handleOpenConnectionRequest2(addr, ch)
	default:
		fmt.Printf("Listener: %v\n", err)
		return nil
	}
}

// Handles an incoming unconnected ping message
func (l *Listener) handleUnconnectedPing(addr *net.UDPAddr, ch chan Wrapper) (err error) {
	msg := message.UnconnectedPing{}
	if err = msg.Read(l.reader); err != nil {
		return
	}

	resp := message.UnconnectedPong{
		SendTimestamp: msg.SendTimestamp,
		ServerGUID:    l.guid,
		Data:          []byte("MCPE;Dedicated Server;390;1.14.60;0;10;13253860892328930865;Bedrock level;Survival;1;19132;19133;"),
	}

	ch <- Wrapper{
		dest: *addr,
		msg:  &resp,
	}

	return
}

// Handles an open connection request 1 message
func (l *Listener) handleOpenConnectionRequest1(addr *net.UDPAddr, ch chan Wrapper) (err error) {
	msg := message.OpenConnectionRequest1{}
	if err = msg.Read(l.reader); err != nil {
		return
	}

	if msg.Protocol != protocol.PROTOCOL_VERSION {
		resp := message.IncompatibleProtocolVersion{
			ServerProtocol: protocol.PROTOCOL_VERSION,
			ServerGUID:     l.guid,
		}

		ch <- Wrapper{
			dest: *addr,
			msg:  &resp,
		}
		return
	}

	mtu := msg.DiscoveringMTU
	if mtu > protocol.MAX_MTU_SIZE || mtu < protocol.MIN_MTU_SIZE {
		mtu = protocol.MAX_MTU_SIZE
	}

	resp := message.OpenConnectionReply1{
		ServerGUID:             l.guid,
		Secure:                 false,
		ServerPreferredMTUSize: uint16(mtu),
	}

	ch <- Wrapper{
		dest: *addr,
		msg:  &resp,
	}

	return
}

// Handles an open connection request 2 message
func (l *Listener) handleOpenConnectionRequest2(addr *net.UDPAddr, ch chan Wrapper) (err error) {
	msg := message.OpenConnectionRequest2{}
	if err = msg.Read(l.reader); err != nil {
		return
	}

	mtu := int(msg.ClientPreferredMTUSize)
	if mtu > protocol.MAX_MTU_SIZE || mtu < protocol.MIN_MTU_SIZE {
		mtu = protocol.MAX_MTU_SIZE
	}

	resp := message.OpenConnectionReply2{
		ServerGUID:    l.guid,
		ClientAddress: *addr,
		MTUSize:       uint16(mtu),
		Secure:        false,
	}

	ch <- Wrapper{
		dest: *addr,
		msg:  &resp,
	}

	l.connections[addr.String()] = newConn(l.addr, addr, l.socket, mtu)
	return
}
