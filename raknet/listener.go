package raknet

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/raknet/internal/message"
	"github.com/gamevidea/raknet/internal/protocol"
)

// DatagramMetrics help in keeping record of the number of datagrams that we receive from a connection in a second
// It is useful for figuring out whether we are being spammed or flooded by large number of datagrams being
// sent by a socket address.
type DatagramMetrics struct {
	timestamp time.Time
	count     int
}

// unconnectedMessage represents an incoming message destined for the listener. It contains the address
// of the source and the buffer in which the message is contained.
type unconnectedMessage struct {
	addr   *net.UDPAddr
	buffer *buffer.Buffer
}

// bufferPool is used for minimising the number of allocations and deallocations as it creates a pool of
// pre-generated buffers which can be taken and given back to allow sharing of same memory. It is dynamically
// scalable as well and overall provides an efficient way of reusing buffers.
var bufferPool = sync.Pool{
	New: func() any {
		return buffer.New(protocol.MAX_MTU_SIZE)
	},
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

	conn chan *Connection
	incm chan unconnectedMessage
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
		conn:              make(chan *Connection),
		incm:              make(chan unconnectedMessage),
	}

	go listener.udpHandler()
	go listener.listenerHandler()

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

// Waits for a new connection to be accepted
func (l *Listener) Accept() *Connection {
	return <-l.conn
}

// Starts a new task that handles a datagram from the udp socket as soon as one is available.
// It sends the datagram through a channel to either the Listener's handler or the connection's handler
// depending upon the peer address it is destined from.
func (l *Listener) udpHandler() {
	for {
		buffer := bufferPool.Get().(*buffer.Buffer)

		len, addr, err := l.socket.ReadFromUDP(buffer.Slice())
		if err != nil {
			fmt.Printf("Socket Read: %v\n", err)
			continue
		}

		buffer.Resize(len)

		if conn, ok := l.connections[addr.String()]; ok {
			if err := conn.readDatagram(buffer); err != nil {
				fmt.Printf("Conn Handle: %v\n", err)
			}

			continue
		}

		l.incm <- unconnectedMessage{
			addr:   addr,
			buffer: buffer,
		}
	}
}

// Starts a task that handles any datagrams destined for the Listener. These datagrams are
// unconnected raknet messages which may be for pinging or for requesting connection.
func (l *Listener) listenerHandler() {
	for {
		msg := <-l.incm

		if err := l.handle(msg); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
	}
}

// Handle is called when an incoming unconnected message is received on the socket. It handles the message
// by flushing the response for the message immediately.
func (l *Listener) handle(incm unconnectedMessage) error {
	id, err := incm.buffer.ReadUint8()
	if err != nil {
		return err
	}

	fmt.Printf("ID: %d\n", id)

	switch id {
	case message.IDUnconnectedPing, message.IDUnconnectedPingOpenConnections:
		return l.handleUnconnectedPing(incm)
	case message.IDOpenConnectionRequest1:
		return l.handleOpenConnectionRequest1(incm)
	case message.IDOpenConnectionRequest2:
		return l.handleOpenConnectionRequest2(incm)
	default:
		fmt.Printf("Unhandled Unconnected ID: %d\n", id)
		return nil
	}
}

// Handles an incoming unconnected ping message
func (l *Listener) handleUnconnectedPing(incm unconnectedMessage) (err error) {
	msg := message.UnconnectedPing{}
	if err = msg.Read(incm.buffer); err != nil {
		return
	}

	incm.buffer.Reset()

	resp := message.UnconnectedPong{
		SendTimestamp: msg.SendTimestamp,
		ServerGUID:    l.guid,
		Data:          []byte("MCPE;Dedicated Server;390;1.14.60;0;10;13253860892328930865;Bedrock level;Survival;1;19132;19133;"),
	}

	if err = resp.Write(incm.buffer); err != nil {
		return err
	}

	if _, err = l.socket.WriteTo(incm.buffer.Bytes(), incm.addr); err != nil {
		return err
	}

	return
}

// Handles an open connection request 1 message
func (l *Listener) handleOpenConnectionRequest1(incm unconnectedMessage) (err error) {
	msg := message.OpenConnectionRequest1{}
	if err = msg.Read(incm.buffer); err != nil {
		return
	}

	incm.buffer.Reset()

	if msg.Protocol != protocol.PROTOCOL_VERSION {
		resp := message.IncompatibleProtocolVersion{
			ServerProtocol: protocol.PROTOCOL_VERSION,
			ServerGUID:     l.guid,
		}

		if err = resp.Write(incm.buffer); err != nil {
			return err
		}

		_, err = l.socket.WriteTo(incm.buffer.Bytes(), incm.addr)
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

	if err = resp.Write(incm.buffer); err != nil {
		return err
	}

	if _, err = l.socket.WriteTo(incm.buffer.Bytes(), incm.addr); err != nil {
		return err
	}

	return
}

// Handles an open connection request 2 message
func (l *Listener) handleOpenConnectionRequest2(incm unconnectedMessage) (err error) {
	msg := message.OpenConnectionRequest2{}
	if err = msg.Read(incm.buffer); err != nil {
		return
	}

	incm.buffer.Reset()

	mtu := int(msg.ClientPreferredMTUSize)
	if mtu > protocol.MAX_MTU_SIZE || mtu < protocol.MIN_MTU_SIZE {
		mtu = protocol.MAX_MTU_SIZE
	}

	resp := message.OpenConnectionReply2{
		ServerGUID:    l.guid,
		ClientAddress: *incm.addr,
		MTUSize:       uint16(mtu),
		Secure:        false,
	}

	if err = resp.Write(incm.buffer); err != nil {
		return err
	}

	if _, err = l.socket.WriteTo(incm.buffer.Bytes(), incm.addr); err != nil {
		return err
	}

	conn := newConn(l.addr, incm.addr, l.socket, mtu)
	go conn.check(l.conn)

	l.connections[incm.addr.String()] = conn
	return
}
