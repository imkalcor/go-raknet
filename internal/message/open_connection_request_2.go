package message

import (
	"net"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// OpenConnectionRequest2 is the next message in the login sequence sent by the client to the server.
// It sends a formulated MTU size from the server preferred MTU size sent by the server in the OpenConnectionReply1 message.
type OpenConnectionRequest2 struct {
	// The address used by the client to connect to the server
	ServerAddress net.UDPAddr

	// Client preferred MTU size formulated from server preferred MTU size
	ClientPreferredMTUSize uint16
	ClientGUID             int64
}

// Reads an open connection request 2 message from the buffer and returns an error if operation
// has failed
func (pk *OpenConnectionRequest2) Read(buf *buffer.Buffer) (err error) {
	if err = buf.ReadMagic(); err != nil {
		return
	}

	if err = buf.ReadAddr(&pk.ServerAddress); err != nil {
		return
	}

	if pk.ClientPreferredMTUSize, err = buf.ReadUint16(byteorder.BigEndian); err != nil {
		return
	}

	if pk.ClientGUID, err = buf.ReadInt64(byteorder.BigEndian); err != nil {
		return
	}

	return
}

// Writes an open connection request 2 to the buffer and returns an error if the operation
// has failed
func (pk *OpenConnectionRequest2) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDOpenConnectionRequest2); err != nil {
		return
	}

	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WriteAddr(&pk.ServerAddress); err != nil {
		return
	}

	if err = buf.WriteUint16(pk.ClientPreferredMTUSize, byteorder.BigEndian); err != nil {
		return
	}

	if err = buf.WriteInt64(pk.ClientGUID, byteorder.BigEndian); err != nil {
		return
	}

	return
}
