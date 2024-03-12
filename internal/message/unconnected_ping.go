package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// UnconnectedPing is sent by the client to query the MCPE related data from the server such as
// MOTD, player count, version, etc.
type UnconnectedPing struct {
	SendTimestamp int64
	ClientGUID    int64
}

// Reads an unconnected ping message from the buffer and returns an error if the operation
// failed.
func (pk *UnconnectedPing) Read(buf buffer.Buffer) (err error) {
	if pk.SendTimestamp, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.ReadMagic(); err != nil {
		return
	}

	if pk.ClientGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	return
}

// Writes an unconnected ping message into the buffer and returns an error if the operation
// failed.
func (pk *UnconnectedPing) Write(buf buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDUnconnectedPing); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.SendTimestamp, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ClientGUID, byteorder.BigEndian); err != nil {
		return
	}

	return
}
