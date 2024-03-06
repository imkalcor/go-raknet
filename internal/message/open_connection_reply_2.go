package message

import (
	"net"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// OpenConnectionReply2 is sent by the server in response to the OpenConnectionRequest2
// thus finalizing the commonly accepted MTU size for the connection.
type OpenConnectionReply2 struct {
	ServerGUID    int64
	ClientAddress net.UDPAddr
	MTUSize       uint16
	Secure        bool
}

// Reads an open connection reply 2 message from the buffer and returns an error if the operation
// has failed
func (pk *OpenConnectionReply2) Read(buf *buffer.Buffer) (err error) {
	if err = buf.ReadMagic(); err != nil {
		return
	}

	if pk.ServerGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.ReadAddr(&pk.ClientAddress); err != nil {
		return
	}

	if pk.MTUSize, err = buf.ReadUint16(byteorder.BigEndian); err != nil {
		return
	}

	if pk.Secure, err = buf.ReadBool(); err != nil {
		return
	}

	return
}

// Writes an open connection reply 2 message to the buffer and returns an error if the operation
// has failed
func (pk *OpenConnectionReply2) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ServerGUID, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteAddr(&pk.ClientAddress); err != nil {
		return
	}

	if err = buf.WriteUint16(pk.MTUSize, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteBool(pk.Secure); err != nil {
		return
	}

	return
}
