package message

import "github.com/gamevidea/binary/buffer"

// ID represents a raknet message ID. It is a unique identifier for each RakNet
// message.
type ID = uint8

const (
	IDUnconnectedPing                ID = 0x01
	IDUnconnectedPingOpenConnections ID = 0x02
	IDUnconnectedPong                ID = 0x1c
	IDOpenConnectionRequest1         ID = 0x05
	IDOpenConnectionReply1           ID = 0x06
	IDOpenConnectionRequest2         ID = 0x07
	IDOpenConnectionReply2           ID = 0x08
	IDIncompatibleProtocolVersion    ID = 0x19
	IDConnectionRequest              ID = 0x09
	IDConnectionRequestAccepted      ID = 0x10
	IDNewIncomingConnection          ID = 0x13
	IDConnectedPing                  ID = 0x00
	IDConnectedPong                  ID = 0x03
	IDDetectLostConnections          ID = 0x04
	IDDisconnectNotification         ID = 0x15
	IDGamePacket                     ID = 0xfe
)

// Message represents a raknet message that may be either connected or unconnected depending upon
// the connection status.
type Message interface {
	Read(buf *buffer.Buffer) (err error)
	Write(buf *buffer.Buffer) (err error)
}
