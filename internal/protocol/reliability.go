package protocol

// Reliability is the type of message ordering, sequencing that a message in raknet can be delivered with.
// MCPE always uses the Reliable Ordered message type.
type Reliability uint8

const (
	Unreliable Reliability = iota
	UnreliableSequenced
	Reliable
	ReliableOrdered
	ReliableSequenced
)

// Returns whether the reliability is of type Reliable.
func (r Reliability) Reliable() bool {
	switch r {
	case Reliable, ReliableOrdered, ReliableSequenced:
		return true
	default:
		return false
	}
}

// Returns whether the reliability is of type sequenced or ordered.
func (r Reliability) SequencedOrdered() bool {
	switch r {
	case ReliableSequenced, UnreliableSequenced, ReliableOrdered:
		return true
	default:
		return false
	}
}

// Returns whether the reliability is of type sequenced
func (r Reliability) Sequenced() bool {
	switch r {
	case ReliableSequenced, UnreliableSequenced:
		return true
	default:
		return false
	}
}
