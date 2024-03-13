package message

import (
	"net"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// NewIncomingConnection is sent by the client to the server in response to the ConnectionRequestAccepted
// message. The next messages are the Game messages.
type NewIncomingConnection struct {
	ServerAddress     net.UDPAddr
	RequestTimestamp  int64
	AcceptedTimestamp int64
}

// Reads a new incoming connection message from the buffer and returns an error if the operation
// has failed
func (pk *NewIncomingConnection) Read(buf *buffer.Buffer) (err error) {
	if err = buf.ReadAddr(&pk.ServerAddress); err != nil {
		return
	}

	if err = buf.ReadSystemAddresses(); err != nil {
		return
	}

	if pk.RequestTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if pk.AcceptedTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	return
}

// Writes a new incoming connection message to the buffer and returns an error if the operation
// has failed
func (pk *NewIncomingConnection) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDNewIncomingConnection); err != nil {
		return
	}

	if err = buf.WriteAddr(&pk.ServerAddress); err != nil {
		return
	}

	if err = buf.WriteSystemAddresses(); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.RequestTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.AcceptedTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	return
}
