package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// ConnectionRequest is the first raknet message sent in an encapsulated frame. It is sent by the
// client to send a request for establishing a connection.
type ConnectionRequest struct {
	ClientGUID       int64
	RequestTimestamp int64

	// For MCPE this field is always sent as false.
	Secure bool
}

// Reads a connection request message from the buffer and returns an error if the operation
// has failed
func (pk *ConnectionRequest) Read(buf *buffer.Buffer) (err error) {
	if pk.ClientGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if pk.RequestTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if pk.Secure, err = buf.ReadBool(); err != nil {
		return
	}

	return
}

// Writes a connection request message to the buffer and returns an error if the operation
// has failed
func (pk *ConnectionRequest) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteInt64(pk.ClientGUID, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.RequestTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteBool(pk.Secure); err != nil {
		return
	}

	return
}
