package protocol

import (
	"time"
)

// SequenceWindow helps in filtering the incoming RakNet datagrams by preventing any datagrams that have
// same sequence number or are out of order from reaching our processing side. It maintains a list of acks
// and nacks that we should flush by the next tick for the sequences we have received and for those we did
// not respectively.
type SequenceWindow struct {
	Start   uint32
	End     uint32
	Highest uint32
	Acks    map[uint32]struct{}
	Nacks   map[uint32]struct{}
}

// Creates and returns a new Sequence Window
func CreateSequenceWindow() SequenceWindow {
	return SequenceWindow{
		Start:   0,
		End:     WINDOW_SIZE,
		Highest: 0,
		Acks:    make(map[uint32]struct{}, WINDOW_SIZE),
		Nacks:   make(map[uint32]struct{}, WINDOW_SIZE),
	}
}

// Receives a sequence number and checks if we have received this sequence before or if it is out of order.
// It returns true if we should continue processing this datagram.
func (w *SequenceWindow) Receive(seq uint32) bool {
	if _, acked := w.Acks[seq]; seq < w.Start || seq > w.End || acked {
		return false
	}

	w.Acks[seq] = struct{}{}
	delete(w.Nacks, seq)

	if seq > w.Highest {
		w.Highest = seq
	}

	if seq == w.Start {
		// got a contiguous packet, shift the receive window
		// this packet might complete a sequence of out-of-order packets, so we incrementally check the indexes
		// to see how far to shift the window, and stop as soon as we either find a gap or have an empty window
		_, acked := w.Acks[w.Start]

		for acked {
			w.Start += 1
			w.End += 1
			_, acked = w.Acks[w.Start]
		}
	} else {
		// we got a gap - a later packet arrived before earlier ones did.
		// we add the earlier ones to the nack queue.
		// if the missing packets arrive before the end of the tick, they'll be removed from nack queue.
		for i := w.Start; i < seq; i++ {
			if _, acked := w.Acks[i]; !acked {
				w.Nacks[i] = struct{}{}
			}
		}
	}

	return true
}

// Shifts the window, this should be called when we should stop expecting a certain set of sequences.
// At this stage, we flush our ACKs and NACKs.
func (w *SequenceWindow) Shift() {
	diff := w.Highest - w.Start

	if diff > 0 {
		w.Start += diff
		w.End += diff
	}
}

// MessageWindow ensures that no datagrams with same message index can reach our processing end. This
// is a second shield from ensuring we don't accidentally handle retransmitted or duplicated datagrams.
// RakNet Datagrams can have unique sequence numbers and have same message index sometimes due to having being
// retransmitted by the other end of the connection if they don't receive ACK for that sequence within a certain
// period of time.
type MessageWindow struct {
	Start   uint32
	End     uint32
	Indexes map[uint32]struct{}
}

// Creates and returns a new message window
func CreateMessageWindow() MessageWindow {
	return MessageWindow{
		Start:   0,
		End:     WINDOW_SIZE,
		Indexes: make(map[uint32]struct{}, WINDOW_SIZE),
	}
}

// Tries to receive a message index and returns whether we should continue processing this datagram or not.
// Returns false if a datagram with the provided message index has already reached us before.
func (w *MessageWindow) Receive(seq uint32) bool {
	if _, recd := w.Indexes[seq]; seq < w.Start || seq > w.End || recd {
		return false
	}

	w.Indexes[seq] = struct{}{}

	if seq == w.Start {
		_, recd := w.Indexes[w.Start]

		for recd {
			w.Start += 1
			w.End += 1

			_, recd = w.Indexes[w.Start]
			delete(w.Indexes, w.Start)
		}
	}

	return true
}

// SplitWindow keeps a record of the fragments we have received so far for a specific raknet message.
// It is used to keep a hold of the fragments until all fragments have been received and assembled together.
type SplitWindow struct {
	Count     uint32
	Fragments map[uint32][]byte
}

// Creates and returns a new split window with the specified capacity
func CreateSplitWindow(count uint32) SplitWindow {
	return SplitWindow{
		Count:     count,
		Fragments: map[uint32][]byte{},
	}
}

// Tries to receive a fragment. Returns nil if one or more fragments are still missing or returns
// the combined slice
func (w *SplitWindow) Receive(index uint32, fragment []byte) []byte {
	w.Fragments[index] = fragment

	if len(w.Fragments) != int(w.Count) {
		return nil
	}

	length := 0
	for _, f := range w.Fragments {
		length += len(f)
	}

	content := make([]byte, 0, length)
	for index, f := range w.Fragments {
		content = append(content, f...)
		delete(w.Fragments, index)
	}

	return content
}

// Record contains information about the datagram that we have sent to the other end of the
// connection. It contains the time at which we sent the datagram which is useful for calculating
// latency, and also contains the encoded bytes that will be useful when retransmitting this datagram.
type Record struct {
	packet    []byte
	timestamp time.Time
}

// RecoveryWindow helps in retransmission of datagrams that the other end of the connection ended up not having
// or for those datagrams that were arrived late and by that time they already sent a NACK for that sequence to us.
// Retransmission also occurs from our end if we don't receive an ACK or a NACK for a certain amount of time.
type RecoveryWindow struct {
	unacknowledged map[uint32]*Record
	delays         map[time.Time]time.Duration
}

// Creates and returns a new recovery window
func CreateRecoveryWindow() RecoveryWindow {
	return RecoveryWindow{
		unacknowledged: map[uint32]*Record{},
		delays:         map[time.Time]time.Duration{},
	}
}

// Adds the datagram's frame data serial to the Recovery Window.
func (w *RecoveryWindow) Add(seq uint32, packet []byte) {
	w.unacknowledged[seq] = &Record{
		packet:    packet,
		timestamp: time.Now(),
	}
}

// Removes the datagram from the recovery window.
func (w *RecoveryWindow) Acknowledge(seq uint32) {
	record, ok := w.unacknowledged[seq]

	if ok {
		w.delays[time.Now()] = time.Since(record.timestamp)
		delete(w.unacknowledged, seq)
	}
}

// Returns the datagram's frame data serialized in bytes if found for the provided
// sequence for retransmission purposes.
func (w *RecoveryWindow) Retransmit(seq uint32) []byte {
	record, ok := w.unacknowledged[seq]

	if ok {
		w.delays[time.Now()] = time.Since(record.timestamp) * 2
		delete(w.unacknowledged, seq)
		return record.packet
	}

	return nil
}

// Returns all the datagrams that were lost during transmission and for which we did not
// receive neither an ACK nor a NACK.
func (w *RecoveryWindow) Lost() [][]byte {
	lost := make([][]byte, 0)

	for seq, pk := range w.unacknowledged {
		if time.Since(pk.timestamp) > TPS {
			lost = append(lost, w.Retransmit(seq))
		}
	}

	return lost
}

// Returns the average time taken by the other end of the connection to acknowledge or NACK
// a sequence. This is also known as latency.
func (w *RecoveryWindow) Rtt() time.Duration {
	const average = time.Second * 5
	var total, records time.Duration
	var now = time.Now()

	for t, rtt := range w.delays {
		if now.Sub(t) > average {
			delete(w.delays, t)
			continue
		}

		total += rtt
		records++
	}

	if records == 0 {
		return time.Millisecond * 50
	}

	return total / records
}
