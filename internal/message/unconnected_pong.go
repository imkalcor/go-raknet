package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// UnconnectedPong is sent by the server in response to the UnconnectedPing message. It sends the server's guid
// and the pong data that contains various information such as MOTD, player count, max player count, server version, etc.
type UnconnectedPong struct {
	SendTimestamp int64
	ServerGUID    int64
	Data          []byte
}

// Reads unconnected pong from the underlying buffer and returns an error if the operation
// failed.
func (pk *UnconnectedPong) Read(buf *buffer.Buffer) (err error) {
	if pk.SendTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if pk.ServerGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.ReadMagic(); err != nil {
		return
	}

	if err = buf.ReadPongData(pk.Data); err != nil {
		return
	}

	return
}

// Writes an Unconnected Pong message to the underlying buffer and returns an error if the operation
// failed.
func (pk *UnconnectedPong) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteInt64(pk.SendTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ServerGUID, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WritePongData(pk.Data); err != nil {
		return
	}

	return
}
