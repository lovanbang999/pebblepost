package streamclient

import (
	"context"
	"errors"
	"sync"

	"pebblepost/internal/types"
)

// StreamEvent wraps either a log entry or a status update for SSE broadcasting to the UI.
type StreamEvent struct {
	Type   string                     `json:"type"` // "log", "status"
	Log    *types.StreamLogEntry      `json:"log,omitempty"`
	Status *types.StreamSessionStatus `json:"status,omitempty"`
}

// StreamSession wraps an active WebSocket or SSE connection and manages broadcaster subscribers.
type StreamSession struct {
	StreamID  string
	Protocol  string // "websocket" or "sse"
	WSClient  *WSClient
	SSEClient *SSEClient

	mu          sync.RWMutex
	subscribers map[chan StreamEvent]struct{}
}

func newStreamSession(streamID, protocol string) *StreamSession {
	return &StreamSession{
		StreamID:    streamID,
		Protocol:    protocol,
		subscribers: make(map[chan StreamEvent]struct{}),
	}
}

// Broadcast dispatches an event to all active listener channels.
func (s *StreamSession) Broadcast(ev StreamEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- ev:
		default:
			// avoid blocking if channel buffer is full
		}
	}
}

// Subscribe registers a new subscriber channel. Returns unsubscribe function.
func (s *StreamSession) Subscribe() (chan StreamEvent, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan StreamEvent, 256)
	s.subscribers[ch] = struct{}{}

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
	}

	return ch, unsubscribe
}

// Status returns the current session status.
func (s *StreamSession) Status() types.StreamSessionStatus {
	if s.WSClient != nil {
		return s.WSClient.Status()
	}
	if s.SSEClient != nil {
		return s.SSEClient.Status()
	}
	return types.StreamSessionStatus{
		StreamID: s.StreamID,
		Protocol: s.Protocol,
		State:    "disconnected",
	}
}

// RingBuffer returns the underlying log ring buffer.
func (s *StreamSession) RingBuffer() *RingBuffer {
	if s.WSClient != nil {
		return s.WSClient.RingBuffer()
	}
	if s.SSEClient != nil {
		return s.SSEClient.RingBuffer()
	}
	return nil
}

// Close closes the underlying client.
func (s *StreamSession) Close(code int, reason string) error {
	if s.WSClient != nil {
		return s.WSClient.Close(code, reason)
	}
	if s.SSEClient != nil {
		return s.SSEClient.Close()
	}
	return nil
}

// Send transmits an outgoing message if session is WebSocket.
func (s *StreamSession) Send(payload, msgType string) error {
	if s.WSClient != nil {
		return s.WSClient.Send(payload, msgType)
	}
	return errors.New("cannot send on SSE session: SSE is unidirectional receive-only")
}

// StreamManager manages all active streaming sessions across PebblePost.
type StreamManager struct {
	mu       sync.RWMutex
	sessions map[string]*StreamSession
}

// DefaultManager is the global singleton stream manager.
var DefaultManager = NewStreamManager()

// NewStreamManager creates a new StreamManager.
func NewStreamManager() *StreamManager {
	return &StreamManager{
		sessions: make(map[string]*StreamSession),
	}
}

// StartWS starts and registers a new WebSocket session.
func (m *StreamManager) StartWS(ctx context.Context, cfg WSClientConfig) (*StreamSession, error) {
	m.mu.Lock()
	if existing, ok := m.sessions[cfg.StreamID]; ok {
		_ = existing.Close(CloseGoingAway, "Replaced by new session")
		delete(m.sessions, cfg.StreamID)
	}

	session := newStreamSession(cfg.StreamID, "websocket")

	userOnLog := cfg.OnLog
	userOnStatus := cfg.OnStatus

	cfg.OnLog = func(entry types.StreamLogEntry) {
		session.Broadcast(StreamEvent{Type: "log", Log: &entry})
		if userOnLog != nil {
			userOnLog(entry)
		}
	}
	cfg.OnStatus = func(status types.StreamSessionStatus) {
		session.Broadcast(StreamEvent{Type: "status", Status: &status})
		if userOnStatus != nil {
			userOnStatus(status)
		}
	}

	client := NewWSClient(cfg)
	session.WSClient = client
	m.sessions[cfg.StreamID] = session
	m.mu.Unlock()

	if err := client.Connect(ctx); err != nil {
		return session, err
	}
	return session, nil
}

// StartSSE starts and registers a new SSE session.
func (m *StreamManager) StartSSE(ctx context.Context, cfg SSEClientConfig) (*StreamSession, error) {
	m.mu.Lock()
	if existing, ok := m.sessions[cfg.StreamID]; ok {
		_ = existing.Close(0, "Replaced by new session")
		delete(m.sessions, cfg.StreamID)
	}

	session := newStreamSession(cfg.StreamID, "sse")

	userOnLog := cfg.OnLog
	userOnStatus := cfg.OnStatus

	cfg.OnLog = func(entry types.StreamLogEntry) {
		session.Broadcast(StreamEvent{Type: "log", Log: &entry})
		if userOnLog != nil {
			userOnLog(entry)
		}
	}
	cfg.OnStatus = func(status types.StreamSessionStatus) {
		session.Broadcast(StreamEvent{Type: "status", Status: &status})
		if userOnStatus != nil {
			userOnStatus(status)
		}
	}

	client := NewSSEClient(cfg)
	session.SSEClient = client
	m.sessions[cfg.StreamID] = session
	m.mu.Unlock()

	if err := client.Connect(ctx); err != nil {
		return session, err
	}
	return session, nil
}

// GetSession returns an active session if found.
func (m *StreamManager) GetSession(streamID string) (*StreamSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[streamID]
	return s, ok
}

// CloseSession terminates and unregisters a session.
func (m *StreamManager) CloseSession(streamID string, code int, reason string) error {
	m.mu.Lock()
	session, ok := m.sessions[streamID]
	if ok {
		delete(m.sessions, streamID)
	}
	m.mu.Unlock()

	if !ok {
		return errors.New("session not found")
	}
	return session.Close(code, reason)
}

// Send sends a message to an active WebSocket session.
func (m *StreamManager) Send(streamID, payload, msgType string) error {
	session, ok := m.GetSession(streamID)
	if !ok {
		return errors.New("session not found or disconnected")
	}
	return session.Send(payload, msgType)
}
