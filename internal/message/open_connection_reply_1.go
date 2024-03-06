package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// OpenConnectionReply1 is sent by the server in response to the OpenConnectionRequest1 message. It sends
// the server guid, and server preferred MTU size that it has formulated from the empty buffer sent by the client
// in OpenConnectionRequest1 packet to discover the MTU size of the connection.
type OpenConnectionReply1 struct {
	ServerGUID             int64
	Secure                 bool
	ServerPreferredMTUSize uint16
}

// Reads an open connection reply 1 message from the buffer and returns an error if the operation
// failed.
func (pk *OpenConnectionReply1) Read(buf *buffer.Buffer) (err error) {
	if err = buf.ReadMagic(); err != nil {
		return
	}

	if pk.ServerGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if pk.Secure, err = buf.ReadBool(); err != nil {
		return
	}

	if pk.ServerPreferredMTUSize, err = buf.ReadUint16(byteorder.BigEndian); err != nil {
		return
	}

	return
}

// Writes an open connection reply 1 message to the underlying buffer and returns an error if
// the operation failed.
func (pk *OpenConnectionReply1) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ServerGUID, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteBool(pk.Secure); err != nil {
		return
	}

	if err = buf.WriteUint16(pk.ServerPreferredMTUSize, byteorder.BigEndian); err != nil {
		return
	}

	return
}
