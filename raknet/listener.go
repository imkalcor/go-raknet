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

// unconnectedMessage represents an outgoing message destined for a remote address. It contains the address
// of the destination and the message to be dispatched.
type unconnectedMessage struct {
	addr *net.UDPAddr
	msg  message.Message
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

	connections sync.Map
	blocked     sync.Map

	datagramMetrics   map[string]*DatagramMetrics
	datagramIntegrity map[string]byte

	dc   chan struct{}
	conn chan *Connection
	send chan unconnectedMessage

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
		connections:       sync.Map{},
		blocked:           sync.Map{},
		datagramMetrics:   map[string]*DatagramMetrics{},
		datagramIntegrity: map[string]byte{},
		dc:                make(chan struct{}),
		conn:              make(chan *Connection),
		send:              make(chan unconnectedMessage),
		reader:            buffer.New(protocol.MAX_MTU_SIZE),
		writer:            buffer.New(protocol.MAX_MTU_SIZE),
	}

	go listener.udpReader()
	go listener.udpWriter()
	go listener.handler()

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

// Blocks the provided address for the specified duration and prevents the listener
// from handling and channeling any packets received from this address.
func (l *Listener) Block(addr string, dur time.Duration) {
	l.blocked.Store(addr, time.Now().Add(dur))

	if metrics, ok := l.datagramMetrics[addr]; ok {
		metrics.count = 0
		metrics.timestamp = time.Now()
	}

	l.datagramIntegrity[addr] = 0
}

// Returns whether the provided address is blocked from the socket
func (l *Listener) Blocked(addr string) bool {
	_, ok := l.blocked.Load(addr)
	return ok
}

// Starts a udp reader task that tries to read any available datagram on the socket and handles it by dispatching
// a response for it.
func (l *Listener) udpReader() {
	for {
		select {
		case <-l.dc:
			return
		default:
			l.reader.Reset()

			len, addr, err := l.socket.ReadFromUDP(l.reader.Slice())
			if err != nil {
				fmt.Printf("Socket Read: %v\n", err)
				continue
			}

			//fmt.Printf("Received %d bytes from %v\n", len, addr)
			l.reader.Resize(len)

			if l.Blocked(addr.String()) {
				continue
			}

			if !l.checkIntegrity(addr.String()) {
				continue
			}

			if !l.checkMetrics(addr.String()) {
				continue
			}

			if conn, ok := l.connections.Load(addr.String()); ok {
				if err := conn.(*Connection).readDatagram(l.reader); err != nil {
					fmt.Printf("Conn Handle: %v\n", err)
				}

				continue
			}

			if err := l.handle(addr); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
		}
	}
}

// Starts a new task that dispatches a message to the specified remote address when one is available
// to be sent.
func (l *Listener) udpWriter() {
	for {
		select {
		case <-l.dc:
			return
		default:
			send := <-l.send

			if err := send.msg.Write(l.writer); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			if _, err := l.socket.WriteTo(l.writer.Bytes(), send.addr); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			l.writer.Reset()
		}
	}
}

// Starts a new task that handles the logic for unblocking the clients whose duration is expired
// and various other raknet related logic
func (l *Listener) handler() {
	for {
		select {
		case <-l.dc:
			l.connections.Range(func(key, value any) bool {
				value.(*Connection).dc <- struct{}{}
				return true
			})
			return
		case <-time.After(protocol.TPS):
			l.blocked.Range(func(key, value any) bool {
				if time.Since(value.(time.Time)) > 0 {
					l.blocked.Delete(key)
				}

				return true
			})

			l.connections.Range(func(key, value any) bool {
				conn := value.(*Connection)

				if conn.state == Disconnected {
					l.connections.Delete(key)
				}

				if time.Since(conn.lastActivity) > protocol.TIMEOUT {
					conn.Disconnect()
				}

				return true
			})
		}
	}
}

// Checks the datagram integrity and returns whether the connection should continue processing
// datagrams received from the provided IP.
func (l *Listener) checkIntegrity(addr string) bool {
	integrity, ok := l.datagramIntegrity[addr]
	if !ok {
		integrity = 0
	}

	if integrity > protocol.MAX_INVALID_MSGS {
		l.Block(addr, protocol.BLOCK_DUR)
		return false
	}

	return true
}

// Checks the datagram metrics and returns whether the connection should continue processing
// datagrams received from the provided IP.
func (l *Listener) checkMetrics(addr string) bool {
	metrics, ok := l.datagramMetrics[addr]
	if !ok || time.Since(metrics.timestamp) > time.Second {
		metrics = &DatagramMetrics{
			timestamp: time.Now(),
			count:     0,
		}
	}

	metrics.count += 1

	if metrics.count > protocol.MAX_MSGS_PER_SEC {
		l.Block(addr, protocol.BLOCK_DUR)
		return false
	}

	return true
}

// handle is called when an incoming unconnected message is received on the socket. It handles the message
// by flushing the response for that message immediately.
func (l *Listener) handle(addr *net.UDPAddr) error {
	id, err := l.reader.ReadUint8()
	if err != nil {
		return err
	}

	//fmt.Printf("ID: %d\n", id)

	switch id {
	case message.IDUnconnectedPing, message.IDUnconnectedPingOpenConnections:
		return l.handleUnconnectedPing(addr)
	case message.IDOpenConnectionRequest1:
		return l.handleOpenConnectionRequest1(addr)
	case message.IDOpenConnectionRequest2:
		return l.handleOpenConnectionRequest2(addr)
	default:
		fmt.Printf("Unhandled Unconnected ID: %d\n", id)
		return nil
	}
}

// Handles an incoming unconnected ping message
func (l *Listener) handleUnconnectedPing(addr *net.UDPAddr) (err error) {
	msg := message.UnconnectedPing{}
	if err = msg.Read(l.reader); err != nil {
		return
	}

	resp := message.UnconnectedPong{
		SendTimestamp: msg.SendTimestamp,
		ServerGUID:    l.guid,
		Data:          []byte("MCPE;Dedicated Server;390;1.14.60;0;10;13253860892328930865;Bedrock level;Survival;1;19132;19133;"),
	}

	l.send <- unconnectedMessage{
		addr: addr,
		msg:  &resp,
	}

	return
}

// Handles an open connection request 1 message
func (l *Listener) handleOpenConnectionRequest1(addr *net.UDPAddr) (err error) {
	msg := message.OpenConnectionRequest1{}
	if err = msg.Read(l.reader); err != nil {
		return
	}

	if msg.Protocol != protocol.PROTOCOL_VERSION {
		resp := message.IncompatibleProtocolVersion{
			ServerProtocol: protocol.PROTOCOL_VERSION,
			ServerGUID:     l.guid,
		}

		l.send <- unconnectedMessage{
			addr: addr,
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

	l.send <- unconnectedMessage{
		addr: addr,
		msg:  &resp,
	}

	return
}

// Handles an open connection request 2 message
func (l *Listener) handleOpenConnectionRequest2(addr *net.UDPAddr) (err error) {
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

	l.send <- unconnectedMessage{
		addr: addr,
		msg:  &resp,
	}

	conn := newConn(l.addr, addr, l.socket, mtu)
	go conn.checkState(l.conn)

	l.connections.Store(addr.String(), conn)
	return
}
