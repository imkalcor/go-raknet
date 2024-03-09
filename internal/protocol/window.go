package protocol

import "slices"

type SequenceWindow struct {
	Start   uint32
	End     uint32
	Highest uint32
	Acks    []uint32
	Nacks   []uint32
}

func CreateSequenceWindow() *SequenceWindow {
	return &SequenceWindow{
		Start:   0,
		End:     WINDOW_SIZE,
		Highest: 0,
		Acks:    make([]uint32, 0, WINDOW_SIZE),
		Nacks:   make([]uint32, 0, WINDOW_SIZE),
	}
}

func (w *SequenceWindow) Receive(seq uint32) bool {
	if seq < w.Start || seq > w.End || slices.Contains(w.Acks, seq) {
		return false
	}

	w.Acks = append(w.Acks, seq)
	w.Nacks = Remove(w.Nacks, seq)

	if seq > w.Highest {
		w.Highest = seq
	}

	if seq == w.Start {
		for slices.Contains(w.Acks, w.Start) {
			w.Start += 1
			w.End += 1
		}
	} else {
		for i := w.Start; i < seq; i++ {
			if !slices.Contains(w.Acks, i) {
				w.Nacks = append(w.Nacks, i)
			}
		}
	}

	return true
}

func (w *SequenceWindow) Shift() {
	diff := w.Highest - w.Start

	if diff > 0 {
		w.Start += diff
		w.End += diff
	}
}

type MessageWindow struct {
	Start   uint32
	End     uint32
	Indexes []uint32
}

func CreateMessageWindow() *MessageWindow {
	return &MessageWindow{
		Start:   0,
		End:     WINDOW_SIZE,
		Indexes: make([]uint32, 0, WINDOW_SIZE),
	}
}

func (w *MessageWindow) Receive(seq uint32) bool {
	if seq < w.Start || seq > w.End || slices.Contains(w.Indexes, seq) {
		return false
	}

	w.Indexes = append(w.Indexes, seq)

	if seq == w.Start {
		for slices.Contains(w.Indexes, w.Start) {
			w.Indexes = Remove(w.Indexes, w.Start)
			w.Start += 1
			w.End += 1
		}
	}

	return true
}

type SplitWindow struct {
	Count     uint32
	Fragments map[uint32][]byte
}

func CreateSplitWindow(count uint32) *SplitWindow {
	return &SplitWindow{
		Count:     count,
		Fragments: map[uint32][]byte{},
	}
}

func (w *SplitWindow) Receive(index uint32, fragment []byte) bool {
	w.Fragments[index] = fragment

	if len(w.Fragments) != int(w.Count) {
		return false
	}

	return true
}

func Remove[T comparable](l []T, item T) []T {
	for i, other := range l {
		if other == item {
			return append(l[:i], l[i+1:]...)
		}
	}
	return l
}
