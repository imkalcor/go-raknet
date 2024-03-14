package raknet

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
	"github.com/gamevidea/raknet/internal/message"
	"github.com/gamevidea/raknet/internal/protocol"
)

// This error is sent when a client tries to login to the server again
var ErrDuplicateLogin = errors.New("the client has tried to login again")

// This error is sent when a buffer does not start with the flag datagram as all
// raknet datagrams must have the datagram flag.
var ErrCorruptDatagram = errors.New("the buffer does not appear to have flag datagram")

// This error is sent when the raknet receipt record has an invalid record type value encoded.
var ErrInvalidRecord = errors.New("an exception has occured while trying to parse the receipt record type")

// This error is sent when a raknet message has invalid length encoded <= 0
var ErrInvalidLength = errors.New("an exception has occured while trying to parse the length (=0)")

// This error is sent when the datagram exceeds the maximum number of frames it is allowed to contain
var ErrMaxBatched = errors.New("the datagram exceeds the maximum number of frames it is allowed to contain")

// This error is sent when the datagram exceeds the maximum number of fragments it is allowed to have
var ErrMaxFragments = errors.New("the datagram exceeds the maximum number of fragments it is allowed to have")

// State represents the connection state of the connection. It is used to check when the
// connection is connecting, connected, disconnected, etc.
type State = uint8

const (
	Connecting State = iota
	Connected
	Disconnected
)

// connectedMessage represents an outgoing messaged destined for the connection's peer address. It
// contains the actual message that need to be dispatched with the reliability that it should be sent
// as.
type connectedMessage struct {
	msg message.Message
	rlb protocol.Reliability
}

// Connection is an established raknet connection stream that handles the reliable encoding and decoding
// of messages, receipts to and from the other end of the connection.
type Connection struct {
	localAddr *net.UDPAddr
	peerAddr  *net.UDPAddr
	socket    *net.UDPConn
	mtu       int

	sequenceWindow protocol.SequenceWindow
	messageWindow  protocol.MessageWindow
	recoveryWindow protocol.RecoveryWindow
	splitWindow    map[uint16]protocol.SplitWindow

	ping         time.Duration
	latency      time.Duration
	lastActivity time.Time
	lastFlushed  time.Time

	receipts map[uint32]struct{}
	state    State

	sequenceNumber uint32
	messageIndex   uint32
	sequenceIndex  uint32
	orderIndex     uint32
	splitID        uint16

	batch  *buffer.Buffer
	buffer *buffer.Buffer

	dc   chan struct{}
	send chan connectedMessage
	retr chan *buffer.Buffer
	game chan []byte
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
		splitWindow:    map[uint16]protocol.SplitWindow{},
		ping:           0,
		latency:        0,
		lastActivity:   time.Now(),
		lastFlushed:    time.Now(),
		receipts:       make(map[uint32]struct{}, protocol.MAX_RECEIPTS),
		state:          Connecting,
		sequenceNumber: 0,
		messageIndex:   0,
		sequenceIndex:  0,
		orderIndex:     0,
		splitID:        0,
		batch:          buffer.New(protocol.MAX_MTU_SIZE),
		buffer:         buffer.New(protocol.MAX_MESSAGE_SIZE),
		dc:             make(chan struct{}),
		send:           make(chan connectedMessage, 250),
		retr:           make(chan *buffer.Buffer, 250),
		game:           make(chan []byte, 250),
	}

	go conn.handler()
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

// Blocks until a game packet is available to be read from the connection and returns it
func (c *Connection) Read() []byte {
	return <-c.game
}

// Writes a game packet to the connection with the reliability Reliabel Ordered.
func (c *Connection) Write(data []byte) {
	msg := message.GamePacket{
		Data: data,
	}

	c.send <- connectedMessage{
		msg: &msg,
		rlb: protocol.ReliableOrdered,
	}
}

// Disconnects the connection. Sends a notification to all worker threads for the connection
// that they should stop. Handles the remaining left packets and sends a disconnect notification.
func (c *Connection) Disconnect() {
	c.dc <- struct{}{}
}

// Checks the connection's state and sends it over to the channel provided once it is ready so that
// it can be retrieved from the Listener upon calling Accept() function.
func (c *Connection) checkState(ch chan *Connection) {
	deadline := time.NewTimer(protocol.TIMEOUT)

	for {
		select {
		case <-c.dc:
			return
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
func (c *Connection) readDatagram(b *buffer.Buffer) error {
	header, err := b.ReadUint8()
	if err != nil {
		return err
	}

	if header == message.IDUnconnectedPing || header == message.IDUnconnectedPingOpenConnections {
		return ErrDuplicateLogin
	}

	if header&protocol.FLAG_DATAGRAM == 0 {
		return ErrCorruptDatagram
	}

	c.lastActivity = time.Now()

	if header&protocol.FLAG_ACK != 0 {
		return c.readAck(b)
	}

	if header&protocol.FLAG_NACK != 0 {
		return c.readNack(b)
	}

	return c.readFrame(b)
}

// Reads an ack receipt destined from the connection's peer address.
func (c *Connection) readAck(b *buffer.Buffer) error {
	if err := c.readReceipts(b); err != nil {
		return err
	}

	for seq := range c.receipts {
		c.recoveryWindow.Acknowledge(seq)
	}

	//fmt.Printf("ACKs: %v\n", c.receipts)
	clear(c.receipts)
	return nil
}

// Reads an nack receipt destined from the connection's peer address.
func (c *Connection) readNack(b *buffer.Buffer) error {
	if err := c.readReceipts(b); err != nil {
		return err
	}

	for seq := range c.receipts {
		c.retr <- c.recoveryWindow.Retransmit(seq)
	}

	//fmt.Printf("NACKs: %v\n", c.receipts)
	clear(c.receipts)
	return nil
}

// Reads an ack or nack receipt from the buffer and returns an error if the operation
// has failed.
func (c *Connection) readReceipts(b *buffer.Buffer) error {
	recordsCount, err := b.ReadInt16(byteorder.BigEndian)
	if err != nil {
		return err
	}

	for i := 0; i < int(recordsCount); i++ {
		recordType, err := b.ReadUint8()
		if err != nil {
			return nil
		}

		switch recordType {
		case protocol.RangedRecord:
			start, err := b.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}

			end, err := b.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}

			for seq := start; seq < end; seq++ {
				c.receipts[seq] = struct{}{}
			}
		case protocol.SingleRecord:
			seq, err := b.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}

			c.receipts[seq] = struct{}{}
		default:
			return ErrInvalidRecord
		}
	}

	return nil
}

// Reads a raknet frame message from the buffer and returns an error if the operation
// has failed.
func (c *Connection) readFrame(b *buffer.Buffer) error {
	seq, err := b.ReadUint24(byteorder.LittleEndian)
	if err != nil {
		return err
	}

	if !c.sequenceWindow.Receive(seq) {
		return nil
	}

	count := 0

	for b.Remaining() != 0 {
		header, err := b.ReadUint8()
		if err != nil {
			return err
		}

		split := (header & protocol.FLAG_FRAGMENTED) != 0
		reliability := protocol.Reliability((header & 224) >> 5)

		length, err := b.ReadUint16(byteorder.BigEndian)
		if err != nil {
			return err
		}

		if length >>= 3; length == 0 {
			return ErrInvalidLength
		}

		var messageIndex uint32

		if reliability.Reliable() {
			messageIndex, err = b.ReadUint24(byteorder.LittleEndian)
			if err != nil {
				return err
			}
		}

		if reliability.Sequenced() {
			b.Shift(3)
		}

		if reliability.SequencedOrdered() {
			b.Shift(3 + 1)
		}

		var splitCount uint32
		var splitID uint16
		var splitIndex uint32

		if split {
			splitCount, err = b.ReadUint32(byteorder.BigEndian)
			if err != nil {
				return err
			}

			splitID, err = b.ReadUint16(byteorder.BigEndian)
			if err != nil {
				return err
			}

			splitIndex, err = b.ReadUint32(byteorder.BigEndian)
			if err != nil {
				return err
			}
		}

		content := make([]byte, length)
		if err := b.Read(content); err != nil {
			return err
		}

		if reliability.Reliable() && !c.messageWindow.Receive(messageIndex) {
			continue
		}

		if split {
			if splitCount > protocol.MAX_FRAGMENT_COUNT {
				return ErrMaxFragments
			}

			splits, ok := c.splitWindow[splitID]
			if !ok {
				splits = protocol.CreateSplitWindow(splitCount)
			}

			if content := splits.Receive(splitIndex, content); content != nil {
				if err := c.readMessage(content); err != nil {
					return err
				}
			}

			c.splitWindow[splitID] = splits
		} else {
			if err := c.readMessage(content); err != nil {
				return err
			}
		}

		count += 1
		if count > protocol.MAX_FRAME_COUNT {
			return ErrMaxBatched
		}
	}

	return nil
}

// Reads a raknet message and returns an error if the operation has failed
func (c *Connection) readMessage(content []byte) error {
	b := buffer.From(content)

	id, err := b.ReadUint8()
	if err != nil {
		return err
	}

	fmt.Printf("ID: %d\n", id)

	switch id {
	case message.IDConnectedPing:
		return c.handleConnectedPing(b)
	case message.IDConnectionRequest:
		return c.handleConnectionRequest(b)
	case message.IDNewIncomingConnection:
		return c.handleNewIncomingConnection(b)
	case message.IDGamePacket:
		msg := message.GamePacket{}
		if err := msg.Read(b); err != nil {
			return err
		}
		c.game <- msg.Data
	case message.IDDetectLostConnections:
		return c.handleDetectLostConnections(b)
	case message.IDDisconnectNotification:
		c.dc <- struct{}{}
	}
	return nil
}

// Handles an incoming connected ping from the peer and returns an error if the operation has failed
func (c *Connection) handleConnectedPing(b *buffer.Buffer) error {
	msg := message.ConnectedPing{}
	if err := msg.Read(b); err != nil {
		return err
	}

	resp := message.ConnectedPong{
		ClientTimestamp: msg.ClientTimestamp,
		ServerTimestamp: msg.ClientTimestamp,
	}

	c.send <- connectedMessage{
		msg: &resp,
		rlb: protocol.Unreliable,
	}

	return nil
}

// Handles an incoming connection request from the peer and returns an error if the operation has failed
func (c *Connection) handleConnectionRequest(b *buffer.Buffer) error {
	msg := message.ConnectionRequest{}
	if err := msg.Read(b); err != nil {
		return err
	}

	resp := message.ConnectionRequestAccepted{
		ClientAddress:     *c.peerAddr,
		RequestTimestamp:  msg.RequestTimestamp,
		AcceptedTimestamp: msg.RequestTimestamp,
	}

	c.send <- connectedMessage{
		msg: &resp,
		rlb: protocol.Unreliable,
	}

	return nil
}

// Handles an incoming connection from the peer and returns an error if the operation has failed
func (c *Connection) handleNewIncomingConnection(b *buffer.Buffer) error {
	msg := message.NewIncomingConnection{}
	if err := msg.Read(b); err != nil {
		return err
	}

	c.state = Connected
	return nil
}

func (c *Connection) handleDetectLostConnections(b *buffer.Buffer) error {
	msg := message.ConnectedPing{}

	c.send <- connectedMessage{
		msg: &msg,
		rlb: protocol.Unreliable,
	}

	return nil
}

// Starts the connection handler that flushes ACKs, NACKs, retransmits datagrams, and covers most of the
// raknet connection logic every raknet TPS.
func (c *Connection) handler() {
	for {
		select {
		case <-c.dc:
			return
		case <-time.After(protocol.TPS):
			/*
			 * Get all the messages in the send queue that need to be dispatched and write
			 * it to the other end of the connection.
			 */
			for i := 0; i < len(c.send); i++ {
				send := <-c.send

				if err := c.writeMessage(send.msg, send.rlb); err != nil {
					fmt.Printf("Transmit: %v\n", err)
					continue
				}
			}

			/*
			 * Get all the messages that need to be retransmitted to the other end of the
			 * connection and flush them to the other end of the connection.
			 */
			for i := 0; i < len(c.retr); i++ {
				buffer := <-c.retr

				defer func() {
					buffer.Reset()
					bufferPool.Put(buffer)
				}()

				if err := c.retransmit(buffer); err != nil {
					fmt.Printf("Retransmit: %v\n", err)
					continue
				}
			}

			/*
			 * If one raknet tick has passed since last flush of the batch and there is some
			 * data in the batch then flush the batch.
			 */
			if time.Since(c.lastFlushed) >= protocol.TPS && c.batch.Offset() > 0 {
				c.flush()
			}

			/*
			 * Shift the sequence window as one tick has passed and write ACKs for those sequences
			 * that we received within last tick and NACKs for those that we did not receive the
			 * acknowledgement for.
			 */
			if len(c.sequenceWindow.Acks) > 0 {
				if err := c.writeAcks(); err != nil {
					fmt.Printf("Ack Write: %v\n", err)
					continue
				}
			}

			if len(c.sequenceWindow.Nacks) > 0 {
				if err := c.writeNacks(); err != nil {
					fmt.Printf("Nack Write: %v\n", err)
					continue
				}
			}

			c.sequenceWindow.Shift()
		}
	}
}

// Writes the ACK receipts to the other end of the connection for those sequences that we have
// received within the last tick. Returns an error if the operation failed.
func (c *Connection) writeAcks() error {
	sequences := make([]uint32, 0, len(c.sequenceWindow.Acks))
	for k := range c.sequenceWindow.Acks {
		sequences = append(sequences, k)
	}

	if err := c.buffer.WriteUint8(protocol.FLAG_DATAGRAM | protocol.FLAG_ACK); err != nil {
		return err
	}

	if err := c.writeReceipts(sequences); err != nil {
		return err
	}

	clear(c.sequenceWindow.Acks)
	return nil
}

// Writes the NACK receipts to the other end of the connection for those sequences that we have
// not received within the last tick. Returns an error if the operation failed.
func (c *Connection) writeNacks() error {
	sequences := make([]uint32, 0, len(c.sequenceWindow.Nacks))
	for k := range c.sequenceWindow.Nacks {
		sequences = append(sequences, k)
	}

	if err := c.buffer.WriteUint8(protocol.FLAG_DATAGRAM | protocol.FLAG_NACK); err != nil {
		return err
	}

	if err := c.writeReceipts(sequences); err != nil {
		return err
	}

	clear(c.sequenceWindow.Nacks)
	return nil
}

// Writes the passed receipts to the other end of the connection. Returns an error if the
// operation has failed.
func (c *Connection) writeReceipts(sequences []uint32) error {
	defer func() {
		c.buffer.Reset()
	}()

	slices.Sort(sequences)
	c.buffer.SetOffset(3)

	first := sequences[0]
	last := sequences[0]
	var recordCount int16 = 0

	for index := range sequences {
		seq := sequences[index]

		if seq == last+1 {
			last = seq

			if index != len(sequences)-1 {
				continue
			}
		}

		if first == last {
			if err := c.buffer.WriteUint8(protocol.SingleRecord); err != nil {
				return err
			}

			if err := c.buffer.WriteUint24(first, byteorder.LittleEndian); err != nil {
				return err
			}
		} else {
			if err := c.buffer.WriteUint8(protocol.RangedRecord); err != nil {
				return err
			}

			if err := c.buffer.WriteUint24(first, byteorder.LittleEndian); err != nil {
				return err
			}

			if err := c.buffer.WriteUint24(last, byteorder.LittleEndian); err != nil {
				return err
			}
		}

		first = seq
		last = seq
		recordCount += 1
	}

	offset := c.buffer.Offset()
	c.buffer.SetOffset(1)

	if err := c.buffer.WriteInt16(recordCount, byteorder.BigEndian); err != nil {
		return err
	}

	c.buffer.SetOffset(offset)

	if _, err := c.socket.WriteTo(c.buffer.Bytes(), c.peerAddr); err != nil {
		return err
	}

	return nil
}

// Writes a message to the raknet connection with the specified reliability and returns an error if the
// operation has failed
func (c *Connection) writeMessage(msg message.Message, reliability protocol.Reliability) error {
	/*
	 * Reset the internal buffer so that it can be reused.
	 */
	defer func() {
		c.buffer.Reset()
	}()

	/*
	 * Encode the message to the internal buffer and split the serialised message
	 * into one or more fragments.
	 */
	if err := msg.Write(c.buffer); err != nil {
		return err
	}

	/*
	 * The order index remains the same throughout all the fragments if any. So does
	 * the split ID.
	 */
	fragments := c.split(c.buffer.Bytes())
	orderIndex := c.orderIndex
	c.orderIndex += 1

	/*
	 * Calculate the fragment count, fragment ID, and other data related to fragments
	 */
	splitCount := uint8(len(fragments))
	splitID := c.splitID
	split := splitCount > 1

	if split {
		c.splitID += 1
	}

	/*
	 * We have to set the offset for the batch to 4th index everytime to allow space
	 * for encoding the sequence number and the datagram header in the end while flushing
	 * the datagram. This is done so as to make transmission & retransmission of datagrams
	 * analogous to each other. Retransmitted datagrams are to be assigned a new sequence number
	 * therefore extracting the logic for encoding the sequence number in the end whenever a
	 * datagram needs to be flushed makes more sense.
	 */
	if c.batch.Offset() < 4 {
		c.batch.SetOffset(4)
	}

	var splitIndex uint8
	for splitIndex = 0; splitIndex < splitCount; splitIndex++ {
		content := fragments[splitIndex]
		max_size := c.batch.Remaining() - protocol.FRAME_BODY_SIZE

		/*
		 * If the content's size cannot be accomodated within the available space in the
		 * current batch then flush the current batch to the socket immediately and reset
		 * the buffer.
		 */
		if len(content) > max_size {
			c.flush()
		}

		header := byte(reliability) << 5
		if split {
			header |= protocol.FLAG_FRAGMENTED
		}

		if err := c.batch.WriteUint8(header); err != nil {
			return err
		}

		if err := c.batch.WriteUint16(uint16(len(content))<<3, byteorder.BigEndian); err != nil {
			return err
		}

		if reliability.Reliable() {
			if err := c.batch.WriteUint24(c.messageIndex, byteorder.LittleEndian); err != nil {
				return err
			}
			c.messageIndex += 1
		}

		if reliability.Sequenced() {
			if err := c.batch.WriteUint24(c.sequenceIndex, byteorder.LittleEndian); err != nil {
				return err
			}
			c.sequenceIndex += 1
		}

		if reliability.SequencedOrdered() {
			if err := c.batch.WriteUint24(orderIndex, byteorder.LittleEndian); err != nil {
				return err
			}

			if err := c.batch.WriteUint8(0); err != nil {
				return err
			}
		}

		if split {
			if err := c.batch.WriteUint32(uint32(splitCount), byteorder.BigEndian); err != nil {
				return err
			}

			if err := c.batch.WriteUint16(splitID, byteorder.BigEndian); err != nil {
				return err
			}

			if err := c.batch.WriteUint32(uint32(splitIndex), byteorder.BigEndian); err != nil {
				return err
			}
		}

		if err := c.batch.Write(content); err != nil {
			return err
		}

		/*
		 * If the reliability type is not Reliable Ordered then we should immediately flush to the
		 * socket as internal messages mostly sent a Unreliable should be sent immediately.
		 */
		if reliability != protocol.ReliableOrdered {
			c.flush()
		}
	}

	return nil
}

// Splits the provided slice of bytes into smaller slices indexed by their order index if it exceeds the
// mtu size of the connection. The sub slices if formed reference the data in the provided original slice
// with 0 memory allocations.
func (c *Connection) split(slice []byte) map[uint8][]byte {
	size := c.mtu - protocol.UDP_HEADER_SIZE - protocol.FRAME_HEADER_SIZE - protocol.FRAME_BODY_SIZE
	len := len(slice)

	if len > size {
		size -= protocol.FRAME_ADDITIONAL_SIZE
	}

	count := len / size
	if len%size != 0 {
		count += 1
	}

	fragments := make(map[uint8][]byte, count)

	for i := 0; i < count; i++ {
		start := i * size
		end := (i + 1) * size

		if end > len {
			end = len
		}

		fragments[uint8(i)] = slice[start:end]
	}

	return fragments
}

// Retransmits the provided datagram to the other end of the connection by encoding a new sequence
// number to it.
func (c *Connection) retransmit(buffer *buffer.Buffer) error {
	/*
	 * Set the offset to 1 to encode the sequence number at the first offset so as to assign
	 * it the new sequence number.
	 */
	offset := buffer.Offset()
	buffer.SetOffset(1)

	if err := buffer.WriteUint24(c.sequenceNumber, byteorder.LittleEndian); err != nil {
		return err
	}

	buffer.SetOffset(offset)

	/*
	 * Add the datagram with new sequence number to the recovery window so that it can be retrieved
	 * in future in case retransmission happens again.
	 */
	c.recoveryWindow.Add(c.sequenceNumber, buffer)
	c.sequenceNumber += 1

	/*
	 * Flush the datagram to the socket and reset the buffer's internal cursor buffer to start from
	 * 4th index so that header can be prepended in the end upon flushing.
	 */
	if _, err := c.socket.WriteTo(buffer.Bytes(), c.peerAddr); err != nil {
		return err
	}

	return nil
}

// Flushes the provided serialised datagram to the other end of the connection. This function may
// either accept a reference to the current batch or to a datagram that needs to be retransmitted.
func (c *Connection) flush() error {
	/*
	 * Update the last flushed time and reset the internal batch buffer so it can be reused.
	 */
	defer func() {
		c.lastFlushed = time.Now()
		c.batch.Reset()
	}()

	/*
	* Reset the offset to 0 to write the header of the datagram such as it's flags
	* and the sequence number which takes 4 bytes. Store the offset for later.
	 */
	offset := c.batch.Offset()
	c.batch.SetOffset(0)

	/*
	 * Encode the datagram header and the flags along with the sequence number
	 */
	if err := c.batch.WriteUint8(protocol.FLAG_DATAGRAM | protocol.FLAG_NEEDS_B_AND_AS); err != nil {
		return err
	}

	if err := c.batch.WriteUint24(c.sequenceNumber, byteorder.LittleEndian); err != nil {
		return err
	}

	/*
	 * Reset the batch's offset to the one we stored earlier and copy the bytes from the batch
	 * to the newly allocated bytes from the bufferPool.
	 */
	c.batch.SetOffset(offset)
	bytes := bufferPool.Get().(*buffer.Buffer)

	if err := bytes.Write(c.batch.Bytes()); err != nil {
		return err
	}

	/*
	 * Add the datagram with new sequence number to the recovery window so that it can be retrieved
	 * in future in case retransmission happens.
	 */
	c.recoveryWindow.Add(c.sequenceNumber, bytes)
	c.sequenceNumber += 1

	/*
	 * Flush the datagram to the socket and reset the buffer's internal cursor buffer to start from
	 * 4th index so that header can be prepended in the end upon flushing.
	 */
	if _, err := c.socket.WriteTo(bytes.Bytes(), c.peerAddr); err != nil {
		return err
	}

	return nil
}
