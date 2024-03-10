package raknet

import (
	"fmt"
	"net"
	"time"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
	"github.com/gamevidea/raknet/internal/message"
	"github.com/gamevidea/raknet/internal/protocol"
)

type State = uint8

const (
	Connecting State = iota
	Connected
	Disconnected
)

// Connection is an established raknet connection stream that handles the reliable encoding and decoding
// of messages, receipts to and from the other end of the connection.
type Connection struct {
	localAddr *net.UDPAddr
	peerAddr  *net.UDPAddr

	socket *net.UDPConn
	mtu    int

	sequenceWindow *protocol.SequenceWindow
	messageWindow  *protocol.MessageWindow
	recoveryWindow *protocol.RecoveryWindow
	splitWindow    map[uint16]*protocol.SplitWindow

	ping         time.Duration
	latency      time.Duration
	lastActivity time.Time

	receipts []uint32
	state    State

	sequenceNumber uint32
	messageIndex   uint32
	sequenceIndex  uint32
	orderIndex     uint32
	splitID        uint16

	msgbuf *buffer.Buffer
	buffer *buffer.Buffer
}

// Creates and returns a new Raknet Connection
func newConn(localAddr *net.UDPAddr, peerAddr *net.UDPAddr, socket *net.UDPConn, mtu int) *Connection {
	conn := &Connection{
		localAddr:      localAddr,
		peerAddr:       peerAddr,
		socket:         socket,
		mtu:            mtu,
		sequenceWindow: protocol.CreateSequenceWindow(),
		messageWindow:  protocol.CreateMessageWindow(),
		recoveryWindow: protocol.CreateRecoveryWindow(),
		splitWindow:    map[uint16]*protocol.SplitWindow{},
		ping:           0,
		latency:        0,
		lastActivity:   time.Now(),
		receipts:       make([]uint32, 0, protocol.MAX_RECEIPTS),
		state:          Connecting,
		msgbuf:         buffer.New(protocol.MAX_MESSAGE_SIZE),
		buffer:         buffer.New(protocol.MAX_MTU_SIZE),
	}

	go conn.startWriteLoop()
	return conn
}

// Returns the local address that the socket on the listener's side is bound to.
func (c *Connection) LocalAddr() *net.UDPAddr {
	return c.localAddr
}

// Returns the peer address of the socket to which the connection is established.
func (c *Connection) PeerAddr() *net.UDPAddr {
	return c.peerAddr
}

// Returns the ping of the connection
func (c *Connection) Ping() time.Duration {
	return c.ping
}

// Returns the latency of the connection
func (c *Connection) Latency() time.Duration {
	return c.latency
}

// Returns the connection state of the connection
func (c *Connection) State() State {
	return c.state
}

// Checks the connection's state and sends it over to the channel provided once it is ready so that
// it can be retrieved from the Listener upon calling Accept() function.
func (c *Connection) check(ch chan *Connection) {
	deadline := time.NewTimer(5 * time.Second)

	for {
		select {
		case <-deadline.C:
			return
		case <-time.After(protocol.TPS):
			if c.state == Connected {
				ch <- c
				return
			}
		}
	}
}

// Reads an incoming datagram on the socket destined from the connection's peer address.
func (c *Connection) readDatagram(reader *buffer.Buffer) error {
	header, err := reader.ReadUint8()
	if err != nil {
		return err
	}

	if header == message.IDUnconnectedPing || header == message.IDUnconnectedPingOpenConnections {
		return DPL_ERROR
	}

	if header&protocol.FLAG_DATAGRAM == 0 {
		return IFD_ERROR
	}

	c.lastActivity = time.Now()

	if header&protocol.FLAG_ACK != 0 {
		return c.readAck(reader)
	}

	if header&protocol.FLAG_NACK != 0 {
		return c.readNack(reader)
	}

	return c.readFrame(reader)
}

// Reads an ack receipt destined from the connection's peer address.
func (c *Connection) readAck(reader *buffer.Buffer) error {
	if err := c.readReceipts(reader); err != nil {
		return err
	}

	for _, seq := range c.receipts {
		c.recoveryWindow.Acknowledge(seq)
	}

	fmt.Printf("ACKs: %v\n", c.receipts)
	clear(c.receipts)
	return nil
}

// Reads an nack receipt destined from the connection's peer address.
func (c *Connection) readNack(reader *buffer.Buffer) error {
	if err := c.readReceipts(reader); err != nil {
		return err
	}

	for _, seq := range c.receipts {
		c.recoveryWindow.Retransmit(seq)
	}

	fmt.Printf("NACKs: %v\n", c.receipts)
	clear(c.receipts)
	return nil
}

// Reads an ack or nack receipt from the buffer and returns an error if the operation
// has failed.
func (c *Connection) readReceipts(reader *buffer.Buffer) error {
	recordsCount, err := reader.ReadInt16(byteorder.BigEndian)
	if err != nil {
		return err
	}

	for i := 0; i < int(recordsCount); i++ {
		recordType, err := reader.ReadUint8()
		if err != nil {
			return nil
		}

		switch recordType {
		case protocol.RangedRecord:
			start, err := reader.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}

			end, err := reader.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}

			for seq := start; seq < end; seq++ {
				c.receipts = append(c.receipts, seq)
			}
		case protocol.SingleRecord:
			seq, err := reader.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}

			c.receipts = append(c.receipts, seq)
		default:
			return IRT_ERROR
		}
	}

	return nil
}

// Reads a raknet frame message from the buffer and returns an error if the operation
// has failed.
func (c *Connection) readFrame(reader *buffer.Buffer) error {
	seq, err := reader.ReadUint24(byteorder.LittleEndian)
	if err != nil {
		return err
	}

	if !c.sequenceWindow.Receive(seq) {
		return nil
	}

	count := 0

	for reader.Remaining() != 0 {
		header, err := reader.ReadUint8()
		if err != nil {
			return err
		}

		split := (header & protocol.FLAG_FRAGMENTED) != 0
		reliability := protocol.Reliability((header & 224) >> 5)

		length, err := reader.ReadUint16(byteorder.BigEndian)
		if err != nil {
			return err
		}

		if length >>= 3; length == 0 {
			return ILN_ERROR
		}

		var messageIndex uint32

		if reliability.Reliable() {
			messageIndex, err = reader.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}
		}

		if reliability.Sequenced() {
			reader.Shift(3)
		}

		if reliability.SequencedOrdered() {
			reader.Shift(3 + 1)
		}

		if reliability.Reliable() && !c.messageWindow.Receive(messageIndex) {
			shift := int(length)

			if split {
				shift += protocol.FRAME_ADDITIONAL_SIZE
			}

			reader.Shift(shift)
			continue
		}

		var splitCount uint32
		var splitID uint16
		var splitIndex uint32

		if split {
			splitCount, err = reader.ReadUint32(byteorder.BigEndian)
			if err != nil {
				return err
			}

			splitID, err = reader.ReadUint16(byteorder.BigEndian)
			if err != nil {
				return err
			}

			splitIndex, err = reader.ReadUint32(byteorder.BigEndian)
			if err != nil {
				return err
			}
		}

		content := make([]byte, length)
		if err := reader.Read(content); err != nil {
			return err
		}

		if split {
			if splitCount > protocol.MAX_FRAGMENT_COUNT {
				return EMF_ERROR
			}

			splits, ok := c.splitWindow[splitID]
			if !ok {
				splits = protocol.CreateSplitWindow(splitCount)
			}

			if c := splits.Receive(splitIndex, content); c != nil {
				content = c
			}

			c.splitWindow[splitID] = splits
		}

		if err := c.readMessage(content); err != nil {
			return err
		}

		count += 1
		if count > protocol.MAX_FRAME_COUNT {
			return MFC_ERROR
		}
	}

	return nil
}

// Reads a raknet message and returns an error if the operation has failed
func (c *Connection) readMessage(buf []byte) error {
	reader := buffer.From(buf)

	id, err := reader.ReadUint8()
	if err != nil {
		return err
	}

	//fmt.Printf("ID: %d\n", id)

	switch id {
	case message.IDConnectedPing:
		return c.handleConnectedPing(reader)
	case message.IDConnectionRequest:
		return c.handleConnectionRequest(reader)
	case message.IDNewIncomingConnection:
		return c.handleNewIncomingConnection(reader)
	}
	return nil
}

// Handles an incoming connected ping from the peer and returns an error if the operation has failed
func (c *Connection) handleConnectedPing(reader *buffer.Buffer) error {
	msg := message.ConnectedPing{}
	if err := msg.Read(reader); err != nil {
		return err
	}

	return nil
}

// Handles an incoming connection request from the peer and returns an error if the operation has failed
func (c *Connection) handleConnectionRequest(reader *buffer.Buffer) error {
	msg := message.ConnectionRequest{}
	if err := msg.Read(reader); err != nil {
		return err
	}

	return nil
}

// Handles an incoming connection from the peer and returns an error if the operation has failed
func (c *Connection) handleNewIncomingConnection(reader *buffer.Buffer) error {
	msg := message.NewIncomingConnection{}
	if err := msg.Read(reader); err != nil {
		return err
	}

	return nil
}

// Starts a write loop that runs periodically every configured Raknet TPS that flushes the
// packet batch we have written so far.
func (c *Connection) startWriteLoop() {
	for {
		select {
		case <-time.After(protocol.TPS):
			if c.buffer.Offset() == 0 {
				continue
			}

			c.flush()
		}
	}
}

// Writes a message to the raknet connection with the specified reliability and returns an error if the
// operation has failed
func (c *Connection) writeMessage(msg message.Message, reliability protocol.Reliability) error {
	if err := msg.Write(c.msgbuf); err != nil {
		return err
	}

	fragments := c.split(c.msgbuf.Slice())
	orderIndex := c.orderIndex
	c.orderIndex += 1

	splitCount := len(fragments)
	splitID := c.splitID
	split := splitCount > 1

	if split {
		c.splitID += 1
	}

	for splitIndex := 0; splitIndex < splitCount; splitIndex++ {
		content := fragments[uint8(splitIndex)]
		max_size := c.buffer.Remaining() - protocol.FRAME_BODY_SIZE

		if len(content) > max_size {
			c.flush()
		}

		header := byte(reliability) << 5
		if split {
			header |= protocol.FLAG_FRAGMENTED
		}

		if err := c.buffer.WriteUint8(header); err != nil {
			return err
		}

		if err := c.buffer.WriteUint16(uint16(len(content))<<3, byteorder.BigEndian); err != nil {
			return err
		}

		if reliability.Reliable() {
			if err := c.buffer.WriteUint24(c.messageIndex, byteorder.LittleEndian); err != nil {
				return err
			}
			c.messageIndex += 1
		}

		if reliability.Sequenced() {
			if err := c.buffer.WriteUint24(c.sequenceIndex, byteorder.LittleEndian); err != nil {
				return err
			}
			c.sequenceIndex += 1
		}

		if reliability.SequencedOrdered() {
			if err := c.buffer.WriteUint24(orderIndex, byteorder.LittleEndian); err != nil {
				return err
			}

			if err := c.buffer.WriteUint8(0); err != nil {
				return err
			}
		}

		if split {
			if err := c.buffer.WriteUint32(uint32(splitCount), byteorder.BigEndian); err != nil {
				return err
			}

			if err := c.buffer.WriteUint16(splitID, byteorder.BigEndian); err != nil {
				return err
			}

			if err := c.buffer.WriteUint32(uint32(splitIndex), byteorder.BigEndian); err != nil {
				return err
			}
		}

		if err := c.buffer.Write(content); err != nil {
			return err
		}

		if reliability != protocol.ReliableOrdered {
			c.flush()
		}
	}

	c.msgbuf.Reset()
	return nil
}

// Splits the provided message slice into smaller fragments if it exceeds the maximum configured MTU size.
func (c *Connection) split(buf []byte) map[uint8][]byte {
	max_mtu := c.mtu - protocol.UDP_HEADER_SIZE - protocol.FRAME_HEADER_SIZE - protocol.FRAME_BODY_SIZE
	len := len(buf)

	if len > max_mtu {
		max_mtu -= protocol.FRAME_ADDITIONAL_SIZE
	}

	count := len / max_mtu
	if len%max_mtu != 0 {
		count += 1
	}

	fragments := make(map[uint8][]byte, count)
	for i := 0; i < count; i++ {
		start := i * max_mtu
		end := start + max_mtu

		if end > len {
			end = len
		}

		fragments[uint8(i)] = buf[start:end]
	}

	return fragments
}

// Flushes the batch of datagrams we have written so far and returns an error if the operation has failed
func (c *Connection) flush() error {
	bytes := make([]byte, c.buffer.Offset())
	copy(bytes, c.buffer.Bytes())

	c.recoveryWindow.Add(c.sequenceNumber, bytes)

	if _, err := c.socket.WriteTo(c.buffer.Bytes(), c.peerAddr); err != nil {
		return err
	}

	c.sequenceNumber += 1
	c.buffer.Reset()

	return nil
}
