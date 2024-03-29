package protocol

import "time"

// This is the Raknet Protocol Version supported by this library
const PROTOCOL_VERSION byte = 11

// This specifies the maximum MTU size that a Raknet Datagram cannot exceed. If it does,
// it will be fragmented.
const MAX_MTU_SIZE int = 1500

// This specifies the minimum MTU size that a Raknet Datagram must have.
const MIN_MTU_SIZE int = 500

// This is the size taken by a raknet message to represent the ID in bytes
const MESSAGE_ID_SIZE int = 1

// This contains the size of the UDP Header.
// IP Header Size (20 bytes)
// UDP header size (8 bytes)
const UDP_HEADER_SIZE int = 20 + 8

// This contains the frame header size.
// Header (uint8)
// Sequence Number (uint24)
const FRAME_HEADER_SIZE int = 1 + 3

// This contains the size of the Frame Body.
// Frame Header (uint8)
// Content Length (int16)
// Message Index (uint24)
// Order Index (uint24)
// Order Channel (uint8)
const FRAME_BODY_SIZE int = 1 + 2 + 3 + 3 + 1

// This contains the additional size of the Frame only if the message is fragmented:
// Fragment Count (int32)
// Fragment ID (int16)
// Fragment Index (int32)
const FRAME_ADDITIONAL_SIZE int = 4 + 2 + 4

// This flag is sent for all the raknet message types including the ACK/NACK receipts.
const FLAG_DATAGRAM uint8 = 0x80

// This flag is set for every frame message. It serves no actual purpose. Sending it or not
// sending it does not make a difference
const FLAG_NEEDS_B_AND_AS uint8 = 0x04

// This flag is set for those datagrams that contain an ACK receipt.
const FLAG_ACK uint8 = 0x40

// This flag is set for those datagrams that contain a NACK receipt.
const FLAG_NACK uint8 = 0x20

// This flag is set for those frame messages that are fragmented into two or more frame messages
const FLAG_FRAGMENTED uint8 = 0x10

// This is the maximum size of a raknet window
const WINDOW_SIZE uint32 = 2048

// This is the number of maximum receipts we can receive in one ACK/NACK message
const MAX_RECEIPTS int = 250

// This is the number of maximum frames that a single raknet datagram can hold
const MAX_FRAME_COUNT int = 250

// This is the number of maximum fragments that a raknet message can have
const MAX_FRAGMENT_COUNT uint32 = 250

// This is the maximum size of a raknet message. Raknet messages cannot exceed this size.
const MAX_MESSAGE_SIZE int = 8000

// TPS is the ticks per second at which various raknet logic such as ACKs, NACKs, and state updates
// are performed.
const TPS = time.Millisecond * 100

// This is the number of maximum messages a raknet connection can send us per second before
// they get blocked
const MAX_MSGS_PER_SEC = 1000

// This is the number of maximum invalid / corrupt messages a raknet connection can send us
// before we block them
const MAX_INVALID_MSGS = 20

// This is the duration for which we should block a bad raknet connection
const BLOCK_DUR = time.Second * 10

// If a raknet connection is not responding for more than this time then it is considered a timeout
const TIMEOUT = time.Second * 5
