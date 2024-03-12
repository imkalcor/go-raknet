package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/binary/byteorder"
)

// ConnectedPing is sent by a connected client to a server to check if the connection
// is still alive. It is used for calculation of ping between the server and the client.
type ConnectedPing struct {
	ClientTimestamp int64
}

// Reads a connected ping message and returns an error if the operation failed
func (pk *ConnectedPing) Read(buf buffer.Buffer) (err error) {
	pk.ClientTimestamp, err = buf.ReadInt64(byteorder.BigEndian)
	return
}

// Writes a connected ping message and returns an error if the operation has failed
func (pk *ConnectedPing) Write(buf buffer.Buffer) (err error) {
	if err = buf.WriteUint8(IDConnectedPing); err != nil {
		return
	}

	err = buf.WriteInt64(pk.ClientTimestamp, byteorder.BigEndian)
	return
}
