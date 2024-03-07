package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// IncompatibleProtocolVersion is sent by the server in response to the OpenConnectionRequest1 message
// if the protocol version sent by the client is outdated or incompatible
type IncompatibleProtocolVersion struct {
	ServerProtocol byte
	ServerGUID     int64
}

// Reads an incompatible protocol version message from the buffer and returns an error if
// the operation has failed
func (pk *IncompatibleProtocolVersion) Read(buf *buffer.Buffer) (err error) {
	if pk.ServerProtocol, err = buf.ReadUint8(); err != nil {
		return
	}

	if err = buf.ReadMagic(); err != nil {
		return
	}

	if pk.ServerGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	return
}

// Writes an incompatible protocol version message to the buffer and returns an error if the operation
// has failed
func (pk *IncompatibleProtocolVersion) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDIncompatibleProtocolVersion); err != nil {
		return
	}

	if err = buf.WriteUint8(pk.ServerProtocol); err != nil {
		return
	}

	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ServerGUID, byteorder.BigEndian); err != nil {
		return
	}

	return
}
