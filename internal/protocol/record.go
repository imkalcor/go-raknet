package protocol

// RecordType specifies the type of record the acknowledgement receipt contains. Record type
// can be either single i.e. one sequence number or ranged i.e. the start and the end of the range.
// Example for ranged record could be: start (12 uint24) - end (18 uint24) containing 6 sequence numbers.
type RecordType = uint8

const (
	SingleRecord RecordType = 0x01
	RangedRecord RecordType = 0x00
)
