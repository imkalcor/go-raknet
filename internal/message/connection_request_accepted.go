package message

import (
	"net"

	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// ConnectionRequestAccepted is sent by the server after accepting the connection request sent
// by the client.
type ConnectionRequestAccepted struct {
	ClientAddress     net.UDPAddr
	RequestTimestamp  int64
	AcceptedTimestamp int64
}

// Reads the connection request accepted message from the buffer and returns an error if the
// operation has failed.
func (pk *ConnectionRequestAccepted) Read(buf buffer.Buffer) (err error) {
	if err = buf.ReadAddr(&pk.ClientAddress); err != nil {
		return
	}

	if err = buf.Shift(2); err != nil {
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

// Writes a connection request accepted message to the underlying buffer and returns an error if
// the operation has failed
func (pk *ConnectionRequestAccepted) Write(buf buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDConnectionRequestAccepted); err != nil {
		return
	}

	if err = buf.WriteAddr(&pk.ClientAddress); err != nil {
		return
	}

	if err = buf.WriteInt16(0, byteorder.BigEndian); err != nil {
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
