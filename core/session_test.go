package core

import (
	"testing"
	"time"
)

func TestSessionManager_NewSession(t *testing.T) {
	sm := NewSessionManager("")

	session := sm.NewSession("user1", "test-session")
	if session == nil {
		t.Fatal("expected session, got nil")
	}

	if session.Name != "test-session" {
		t.Errorf("expected name 'test-session', got %q", session.Name)
	}

	if session.ID != "s1" {
		t.Errorf("expected ID 's1', got %q", session.ID)
	}
}

func TestSessionManager_GetOrCreateActive(t *testing.T) {
	sm := NewSessionManager("")

	// First call creates a new session
	session1 := sm.GetOrCreateActive("user1")
	if session1 == nil {
		t.Fatal("expected session, got nil")
	}

	// Second call returns the same session
	session2 := sm.GetOrCreateActive("user1")
	if session2.ID != session1.ID {
		t.Errorf("expected same session, got different: %s vs %s", session1.ID, session2.ID)
	}
}

func TestSessionManager_SwitchSession(t *testing.T) {
	sm := NewSessionManager("")

	// Create two sessions
	sm.NewSession("user1", "session-1")
	sm.NewSession("user1", "session-2")

	// Switch to session-1
	session, err := sm.SwitchSession("user1", "session-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if session.Name != "session-1" {
		t.Errorf("expected session-1, got %s", session.Name)
	}

	// Switch by ID
	session2, err := sm.SwitchSession("user1", "s2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if session2.Name != "session-2" {
		t.Errorf("expected session-2, got %s", session2.Name)
	}

	// Switch to non-existent session
	_, err = sm.SwitchSession("user1", "non-existent")
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

func TestSessionManager_ListSessions(t *testing.T) {
	sm := NewSessionManager("")

	sm.NewSession("user1", "session-1")
	sm.NewSession("user1", "session-2")
	sm.NewSession("user1", "session-3")

	sessions := sm.ListSessions("user1")
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestSession_TryLock(t *testing.T) {
	s := &Session{ID: "s1", Name: "test"}

	// First lock should succeed
	if !s.TryLock() {
		t.Error("expected TryLock to succeed")
	}

	// Second lock should fail (same goroutine, not unlocked)
	if s.TryLock() {
		t.Error("expected TryLock to fail while locked")
	}

	s.Unlock()

	// After unlock, should succeed again
	if !s.TryLock() {
		t.Error("expected TryLock to succeed after unlock")
	}
}

func TestSession_AddHistory(t *testing.T) {
	s := &Session{ID: "s1", Name: "test"}

	s.AddHistory("user", "Hello")
	s.AddHistory("assistant", "Hi there")

	if len(s.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(s.History))
	}

	if s.History[0].Role != "user" {
		t.Errorf("expected first entry role 'user', got %q", s.History[0].Role)
	}

	if s.History[0].Content != "Hello" {
		t.Errorf("expected first entry content 'Hello', got %q", s.History[0].Content)
	}
}

func TestSession_GetHistory(t *testing.T) {
	s := &Session{ID: "s1", Name: "test"}

	s.AddHistory("user", "msg1")
	s.AddHistory("assistant", "msg2")
	s.AddHistory("user", "msg3")
	s.AddHistory("assistant", "msg4")

	// Get last 2
	history := s.GetHistory(2)
	if len(history) != 2 {
		t.Errorf("expected 2 entries, got %d", len(history))
	}
	if history[0].Content != "msg3" {
		t.Errorf("expected msg3, got %q", history[0].Content)
	}
	if history[1].Content != "msg4" {
		t.Errorf("expected msg4, got %q", history[1].Content)
	}

	// Get all (n <= 0)
	all := s.GetHistory(0)
	if len(all) != 4 {
		t.Errorf("expected 4 entries, got %d", len(all))
	}
}

func TestSession_ClearHistory(t *testing.T) {
	s := &Session{ID: "s1", Name: "test"}

	s.AddHistory("user", "msg1")
	s.AddHistory("assistant", "msg2")

	s.ClearHistory()

	if len(s.History) != 0 {
		t.Errorf("expected empty history after clear, got %d entries", len(s.History))
	}
}

func TestSessionManager_ActiveSessionID(t *testing.T) {
	sm := NewSessionManager("")

	// No session yet
	id := sm.ActiveSessionID("user1")
	if id != "" {
		t.Errorf("expected empty ID, got %q", id)
	}

	// Create session
	sm.GetOrCreateActive("user1")

	id = sm.ActiveSessionID("user1")
	if id == "" {
		t.Error("expected non-empty ID after creating session")
	}
}

func TestSession_UpdatedAt(t *testing.T) {
	sm := NewSessionManager("")

	session := sm.NewSession("user1", "test")
	createdAt := session.CreatedAt

	time.Sleep(10 * time.Millisecond)

	session.AddHistory("user", "test message")
	session.Unlock() // This should update UpdatedAt

	if session.UpdatedAt.Before(createdAt) {
		t.Error("expected UpdatedAt to be after CreatedAt")
	}
}
