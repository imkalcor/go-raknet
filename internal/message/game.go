package message

import "github.com/gamevidea/binary/buffer"

// GamePacket represents a compressed and encrypted Minecraft Packet batch.
type GamePacket struct {
	Data []byte
}

// Reads a game packet from the other end of the connection. Returns an error if the operation
// has failed.
func (pk *GamePacket) Read(buf *buffer.Buffer) (err error) {
	if buf.Remaining() < 1 {
		return buffer.ErrEndOfFile
	}

	pk.Data = buf.Slice()[1:]
	return nil
}

// Writes the game packet to the other end of the connection. Returns an error if the operation
// has failed.
func (pk *GamePacket) Write(buf *buffer.Buffer) (err error) {
	if err = buf.Write(pk.Data); err != nil {
		return err
	}

	return nil
}
