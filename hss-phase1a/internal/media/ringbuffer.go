package media

import (
	"sync"
)

// RingBuffer provides a fixed-capacity, thread-safe circular buffer for MediaPackets.
// It is pre-allocated at initialization to avoid heap allocations along the hot RTP ingest path
// and supports instant pre-roll GOP extraction when a viewer connects or a fire alarm triggers.
type RingBuffer struct {
	mu       sync.RWMutex
	buf      []MediaPacket
	capacity int
	head     int
	size     int
	totalIn  uint64
}

// NewRingBuffer creates a pre-allocated circular buffer with the specified packet capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 256
	}
	return &RingBuffer{
		buf:      make([]MediaPacket, capacity),
		capacity: capacity,
	}
}

// Push inserts a MediaPacket into the circular buffer, overwriting the oldest entry when full.
func (rb *RingBuffer) Push(pkt MediaPacket) {
	rb.mu.Lock()
	rb.buf[rb.head] = pkt
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
	rb.totalIn++
	rb.mu.Unlock()
}

// Snapshot returns a chronological slice of all currently buffered MediaPackets (oldest to newest).
func (rb *RingBuffer) Snapshot() []MediaPacket {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.size == 0 {
		return nil
	}

	out := make([]MediaPacket, rb.size)
	start := (rb.head - rb.size + rb.capacity) % rb.capacity
	for i := 0; i < rb.size; i++ {
		idx := (start + i) % rb.capacity
		out[i] = rb.buf[idx]
	}
	return out
}

// SnapshotFromLatestKeyFrame returns all packets starting from the most recent IDR KeyFrame
// up to the newest packet, enabling instant decoder initialization and pre-event clip extraction.
func (rb *RingBuffer) SnapshotFromLatestKeyFrame() []MediaPacket {
	snap := rb.Snapshot()
	if len(snap) == 0 {
		return nil
	}

	lastKFIdx := -1
	for i := len(snap) - 1; i >= 0; i-- {
		if snap[i].IsKeyFrame {
			lastKFIdx = i
			break
		}
	}

	if lastKFIdx == -1 {
		return snap
	}
	return snap[lastKFIdx:]
}

// Len returns the current number of buffered packets.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size
}

// Cap returns the maximum capacity of the ring buffer.
func (rb *RingBuffer) Cap() int {
	return rb.capacity
}

// TotalPushed returns the monotonic count of packets pushed since creation.
func (rb *RingBuffer) TotalPushed() uint64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.totalIn
}

// Reset clears the buffer state without reallocating the backing slice.
func (rb *RingBuffer) Reset() {
	rb.mu.Lock()
	rb.head = 0
	rb.size = 0
	rb.mu.Unlock()
}
