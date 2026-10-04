package streamclient

import (
	"sync"

	"pebblepost/internal/types"
)

// RingBuffer stores a bounded list of StreamLogEntry items with both count and byte caps.
type RingBuffer struct {
	mu           sync.RWMutex
	maxEntries   int
	maxBytes     int64
	entries      []types.StreamLogEntry
	currentBytes int64
	evictedCount int
}

// NewRingBuffer creates a new RingBuffer.
// If maxEntries <= 0, a default of 1000 is used.
// If maxBytes <= 0, a default of 5 MB (5*1024*1024) is used.
func NewRingBuffer(maxEntries int, maxBytes int64) *RingBuffer {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	if maxBytes <= 0 {
		maxBytes = 5 * 1024 * 1024
	}
	return &RingBuffer{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		entries:    make([]types.StreamLogEntry, 0, min(maxEntries, 256)),
	}
}

// Push adds an entry to the ring buffer, evicting oldest entries if caps are exceeded.
func (rb *RingBuffer) Push(entry types.StreamLogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	entryBytes := int64(entry.Size)
	if entryBytes <= 0 {
		entryBytes = int64(len(entry.Payload))
	}

	rb.entries = append(rb.entries, entry)
	rb.currentBytes += entryBytes

	// Evict while exceeding maxEntries or maxBytes
	for (len(rb.entries) > rb.maxEntries || (rb.currentBytes > rb.maxBytes && len(rb.entries) > 1)) && len(rb.entries) > 0 {
		evicted := rb.entries[0]
		eBytes := int64(evicted.Size)
		if eBytes <= 0 {
			eBytes = int64(len(evicted.Payload))
		}
		rb.entries = rb.entries[1:]
		rb.currentBytes -= eBytes
		if rb.currentBytes < 0 {
			rb.currentBytes = 0
		}
		rb.evictedCount++
	}
}

// GetAll returns a copy of all current entries in chronological order.
func (rb *RingBuffer) GetAll() []types.StreamLogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]types.StreamLogEntry, len(rb.entries))
	copy(result, rb.entries)
	return result
}

// Count returns the number of active entries currently retained in the buffer.
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return len(rb.entries)
}

// EvictedCount returns the total number of entries evicted since creation or last clear.
func (rb *RingBuffer) EvictedCount() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.evictedCount
}

// TotalBytes returns the estimated byte size of entries currently stored.
func (rb *RingBuffer) TotalBytes() int64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.currentBytes
}

// Clear resets the buffer.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.entries = rb.entries[:0]
	rb.currentBytes = 0
	rb.evictedCount = 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
