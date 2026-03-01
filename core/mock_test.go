package core

import (
	"context"
	"sync"
)

// MockPlatform implements Platform interface for testing.
type MockPlatform struct {
	NameValue     string
	Messages      []Message
	MessagesMu    sync.Mutex
	ReplyContents []string
	StartFunc     func(handler MessageHandler) error
	StopFunc      func() error
}

func NewMockPlatform(name string) *MockPlatform {
	return &MockPlatform{
		NameValue:     name,
		Messages:      make([]Message, 0),
		ReplyContents: make([]string, 0),
	}
}

func (m *MockPlatform) Name() string {
	return m.NameValue
}

func (m *MockPlatform) Start(handler MessageHandler) error {
	if m.StartFunc != nil {
		return m.StartFunc(handler)
	}
	return nil
}

func (m *MockPlatform) Reply(ctx context.Context, replyCtx any, content string) error {
	m.MessagesMu.Lock()
	defer m.MessagesMu.Unlock()
	m.ReplyContents = append(m.ReplyContents, content)
	return nil
}

func (m *MockPlatform) Send(ctx context.Context, replyCtx any, content string) error {
	m.MessagesMu.Lock()
	defer m.MessagesMu.Unlock()
	m.ReplyContents = append(m.ReplyContents, content)
	return nil
}

func (m *MockPlatform) Stop() error {
	if m.StopFunc != nil {
		return m.StopFunc()
	}
	return nil
}

func (m *MockPlatform) GetReplies() []string {
	m.MessagesMu.Lock()
	defer m.MessagesMu.Unlock()
	result := make([]string, len(m.ReplyContents))
	copy(result, m.ReplyContents)
	return result
}

func (m *MockPlatform) Reset() {
	m.MessagesMu.Lock()
	defer m.MessagesMu.Unlock()
	m.Messages = make([]Message, 0)
	m.ReplyContents = make([]string, 0)
}

// MockAgent implements Agent interface for testing.
type MockAgent struct {
	NameValue      string
	Sessions       map[string]*MockAgentSession
	SessionsMu     sync.Mutex
	StartSessionFn func(ctx context.Context, sessionID string) (AgentSession, error)
	ListSessionsFn func(ctx context.Context) ([]AgentSessionInfo, error)
	StopFunc       func() error
}

func NewMockAgent(name string) *MockAgent {
	return &MockAgent{
		NameValue: name,
		Sessions:  make(map[string]*MockAgentSession),
	}
}

func (m *MockAgent) Name() string {
	return m.NameValue
}

func (m *MockAgent) StartSession(ctx context.Context, sessionID string) (AgentSession, error) {
	m.SessionsMu.Lock()
	defer m.SessionsMu.Unlock()

	if m.StartSessionFn != nil {
		return m.StartSessionFn(ctx, sessionID)
	}

	session := NewMockAgentSession(sessionID)
	m.Sessions[sessionID] = session
	return session, nil
}

func (m *MockAgent) ListSessions(ctx context.Context) ([]AgentSessionInfo, error) {
	if m.ListSessionsFn != nil {
		return m.ListSessionsFn(ctx)
	}
	return []AgentSessionInfo{}, nil
}

func (m *MockAgent) Stop() error {
	if m.StopFunc != nil {
		return m.StopFunc()
	}
	return nil
}

// MockAgentSession implements AgentSession interface for testing.
type MockAgentSession struct {
	sessionID string
	events    chan Event
	closed    bool
	mu        sync.Mutex
}

func NewMockAgentSession(sessionID string) *MockAgentSession {
	return &MockAgentSession{
		sessionID: sessionID,
		events:    make(chan Event, 100),
	}
}

func (m *MockAgentSession) Send(prompt string, images []ImageAttachment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	// Emit a text event as response
	m.events <- Event{
		Type:      EventText,
		Content:   "Mock response: " + prompt,
		SessionID: m.sessionID,
	}
	m.events <- Event{
		Type:      EventResult,
		Content:   "Mock final response for: " + prompt,
		SessionID: m.sessionID,
		Done:      true,
	}
	return nil
}

func (m *MockAgentSession) RespondPermission(requestID string, result PermissionResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	return nil
}

func (m *MockAgentSession) Events() <-chan Event {
	return m.events
}

func (m *MockAgentSession) CurrentSessionID() string {
	return m.sessionID
}

func (m *MockAgentSession) Alive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.closed
}

func (m *MockAgentSession) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		close(m.events)
		m.closed = true
	}
	return nil
}

// EmitEvent allows tests to manually emit events from the mock session.
func (m *MockAgentSession) EmitEvent(event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.events <- event
	}
}

// Asserts that MockAgent implements Agent interface.
var _ Agent = (*MockAgent)(nil)

// Asserts that MockPlatform implements Platform interface.
var _ Platform = (*MockPlatform)(nil)

// Asserts that MockAgentSession implements AgentSession interface.
var _ AgentSession = (*MockAgentSession)(nil)
