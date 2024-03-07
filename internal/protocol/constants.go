package protocol

// This is the Raknet Protocol Version supported by this library
const PROTOCOL_VERSION byte = 11

// This specifies the maximum MTU size that a Raknet Datagram cannot exceed. If it does,
// it will be fragmented.
const MAX_MTU_SIZE uint16 = 1500

// This specifies the minimum MTU size that a Raknet Datagram must have.
const MIN_MTU_SIZE uint16 = 500

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

// This contains the additional size of the Frame only if the packet is fragmented:
// Fragment Count (int32)
// Fragment ID (int16)
// Fragment Index (int32)
const FRAME_ADDITIONAL_SIZE int = 4 + 2 + 4
