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
	}

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
			// sequence index (uint24)
			reader.Shift(3)
		}

		if reliability.SequencedOrdered() {
			// order index (uint24)
			// order channel (uint8)
			reader.Shift(3 + 1)
		}

		if !c.messageWindow.Receive(messageIndex) {
			shift := int(length)

			if split {
				// split count (int32)
				// split ID (int16)
				// split index (int32)
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

func (c *Connection) readMessage(buf []byte) error {
	reader := buffer.From(buf)

	_, err := reader.ReadUint8()
	if err != nil {
		return err
	}

	//fmt.Printf("ID: %d\n", id)
	return nil
}
