package message

import "github.com/gamevidea/binary/buffer"

// Disconnect can be used to disconnect a raknet connection. It can be sent by either the client
// or the server
type Disconnect struct{}

// Reads a disconnect message from the buffer and returns an error if the operation
// has failed
func (pk *Disconnect) Read(buffer *buffer.Buffer) (err error) {
	return
}

// Writes a disconnect message to the buffer and returns an error if the operation
// has failed
func (pk *Disconnect) Write(buffer *buffer.Buffer) (err error) {
	err = buffer.WriteUint8(IDDisconnectNotification)
	return
}
