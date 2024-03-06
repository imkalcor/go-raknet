package message

import (
	"github.com/gamevidea/binary/buffer"
	"github.com/gamevidea/raknet/internal/protocol"
)

// OpenConnectionRequest1 is the first packet sent by the client in the login sequence for raknet. It contains
// zero padding at the end of the packet to make the size of the packet reach a certain size known as the DiscoveringMTU.
// It is sent to know the maximum size of datagram that the network and the destination raknet server can handle.
type OpenConnectionRequest1 struct {
	Protocol byte

	// DiscoveringMTU is the total size of the datagram sent over UDP network that doesn't get dropped.
	DiscoveringMTU int
}

// Reads an open connection request 1 from the buffer and returns an error if the operation
// failed.
func (pk *OpenConnectionRequest1) Read(buf *buffer.Buffer) (err error) {
	pk.DiscoveringMTU = protocol.UDP_HEADER_SIZE + protocol.MESSAGE_ID_SIZE + buf.Remaining()

	if err = buf.ReadMagic(); err != nil {
		return
	}

	if pk.Protocol, err = buf.ReadUint8(); err != nil {
		return
	}

	return
}

// Writes an open connection request 1 into the buffer and returns an error if the operation
// failed.
func (pk *OpenConnectionRequest1) Write(buf *buffer.Buffer) (err error) {
	if err = buf.WriteMagic(); err != nil {
		return
	}

	if err = buf.WriteUint8(pk.Protocol); err != nil {
		return
	}

	remaining := pk.DiscoveringMTU - protocol.UDP_HEADER_SIZE - protocol.MESSAGE_ID_SIZE
	emptyBuf := make([]byte, remaining)

	if err = buf.Write(emptyBuf); err != nil {
		return
	}

	return
}
