package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// ConnectedPong is sent by the server in response to the ConnectedPing message.
// It contains the original client timestamp sent by the client in the ping message and the
// server's timestamp which can be used for calculating the ping.
type ConnectedPong struct {
	ClientTimestamp int64
	ServerTimestamp int64
}

// Reads a connected pong message from the buffer and returns an error if the operation
// has failed
func (pk *ConnectedPong) Read(buf *buffer.Buffer) (err error) {
	if pk.ClientTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if pk.ServerTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	return
}

// Writes a connected pong message from the buffer and returns an error if the operation
// has failed
func (pk *ConnectedPong) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDConnectedPong); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ClientTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ServerTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	return
}
