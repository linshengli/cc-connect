package channel

import (
	"testing"
	"time"
)

func TestManager_New(t *testing.T) {
	m := NewManager(10)

	if m == nil {
		t.Fatal("expected manager")
	}

	if m.maxGlobal != 10 {
		t.Errorf("expected maxGlobal=10, got %d", m.maxGlobal)
	}
}

func TestManager_GetOrCreateChannel(t *testing.T) {
	m := NewManager(10)

	ch := m.GetOrCreateChannel("feishu", "user123", "Test User")

	if ch == nil {
		t.Fatal("expected channel")
	}

	if ch.Config.Platform != "feishu" {
		t.Errorf("expected platform feishu, got %s", ch.Config.Platform)
	}

	if ch.Config.UserID != "user123" {
		t.Errorf("expected userID user123, got %s", ch.Config.UserID)
	}

	// Get same channel again
	ch2 := m.GetOrCreateChannel("feishu", "user123", "Test User")
	if ch != ch2 {
		t.Error("expected same channel instance")
	}
}

func TestManager_GetChannel(t *testing.T) {
	m := NewManager(10)

	m.GetOrCreateChannel("slack", "user456", "Slack User")

	ch, ok := m.GetChannel("slack", "user456")
	if !ok {
		t.Fatal("expected to find channel")
	}

	if ch.Config.UserName != "Slack User" {
		t.Errorf("expected user name 'Slack User', got %s", ch.Config.UserName)
	}

	// Non-existent channel
	_, ok = m.GetChannel("slack", "nonexistent")
	if ok {
		t.Error("expected not to find nonexistent channel")
	}
}

func TestManager_RegisterSession(t *testing.T) {
	m := NewManager(10)

	m.GetOrCreateChannel("telegram", "user789", "TG User")
	m.RegisterSession("session_abc", "telegram", "user789")

	ch, ok := m.GetChannelBySession("session_abc")
	if !ok {
		t.Fatal("expected to find channel by session")
	}

	if ch.Config.UserID != "user789" {
		t.Errorf("expected userID user789, got %s", ch.Config.UserID)
	}

	// Non-existent session
	_, ok = m.GetChannelBySession("nonexistent")
	if ok {
		t.Error("expected not to find channel for nonexistent session")
	}
}

func TestChannel_CanAccept(t *testing.T) {
	cfg := &ChannelConfig{
		ID:        "test",
		RateLimit: 5,
	}
	ch := NewChannel(cfg)

	// Should accept initially
	for i := 0; i < 5; i++ {
		if !ch.CanAccept() {
			t.Errorf("expected to accept request %d", i+1)
		}
		ch.RecordRequest()
	}

	// Should reject after limit
	if ch.CanAccept() {
		t.Error("expected to reject after rate limit")
	}
}

func TestChannel_State(t *testing.T) {
	ch := NewChannel(&ChannelConfig{ID: "test"})

	if ch.GetState() != ChannelActive {
		t.Error("expected initial state active")
	}

	ch.SetState(ChannelPaused)
	if ch.GetState() != ChannelPaused {
		t.Error("expected state paused")
	}

	ch.SetState(ChannelClosed)
	if ch.GetState() != ChannelClosed {
		t.Error("expected state closed")
	}
}

func TestChannel_CanAccept_WhenPaused(t *testing.T) {
	ch := NewChannel(&ChannelConfig{ID: "test"})

	ch.SetState(ChannelPaused)

	if ch.CanAccept() {
		t.Error("expected paused channel to reject")
	}
}

func TestManager_Acquire(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")

	ctx, cancel, err := m.Acquire("feishu", "user1")
	if err != nil {
		t.Fatalf("expected to acquire slot: %v", err)
	}

	if ctx == nil {
		t.Fatal("expected context")
	}

	if cancel == nil {
		t.Fatal("expected cancel function")
	}

	cancel()
	m.Release()
}

func TestManager_Acquire_GlobalLimit(t *testing.T) {
	m := NewManager(2)

	m.GetOrCreateChannel("feishu", "user1", "User 1")
	m.GetOrCreateChannel("feishu", "user2", "User 2")
	m.GetOrCreateChannel("feishu", "user3", "User 3")

	// Acquire max slots
	_, cancel1, _ := m.Acquire("feishu", "user1")
	defer func() { cancel1(); m.Release() }()

	_, cancel2, _ := m.Acquire("feishu", "user2")
	defer func() { cancel2(); m.Release() }()

	// Third should fail
	_, _, err := m.Acquire("feishu", "user3")
	if err == nil {
		t.Error("expected error when global limit reached")
	}
}

func TestManager_Acquire_ChannelNotFound(t *testing.T) {
	m := NewManager(5)

	_, _, err := m.Acquire("feishu", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent channel")
	}
}

func TestManager_PauseResume(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")

	// Pause
	err := m.PauseChannel("feishu", "user1")
	if err != nil {
		t.Fatalf("PauseChannel failed: %v", err)
	}

	ch, _ := m.GetChannel("feishu", "user1")
	if ch.GetState() != ChannelPaused {
		t.Error("expected channel paused")
	}

	// Resume
	err = m.ResumeChannel("feishu", "user1")
	if err != nil {
		t.Fatalf("ResumeChannel failed: %v", err)
	}

	if ch.GetState() != ChannelActive {
		t.Error("expected channel active after resume")
	}
}

func TestManager_CloseChannel(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")
	m.RegisterSession("session_1", "feishu", "user1")

	err := m.CloseChannel("feishu", "user1")
	if err != nil {
		t.Fatalf("CloseChannel failed: %v", err)
	}

	// Should not find channel anymore
	_, ok := m.GetChannel("feishu", "user1")
	if ok {
		t.Error("expected channel to be closed")
	}

	// Session index should also be cleaned
	_, ok = m.GetChannelBySession("session_1")
	if ok {
		t.Error("expected session index to be cleaned")
	}
}

func TestManager_ListChannels(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")
	m.GetOrCreateChannel("slack", "user2", "User 2")
	m.GetOrCreateChannel("telegram", "user3", "User 3")

	list := m.ListChannels()

	if len(list) != 3 {
		t.Errorf("expected 3 channels, got %d", len(list))
	}
}

func TestManager_GetStats(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")
	m.GetOrCreateChannel("slack", "user2", "User 2")

	m.PauseChannel("slack", "user2")

	stats := m.GetStats()

	if stats.TotalChannels != 2 {
		t.Errorf("expected 2 total channels, got %d", stats.TotalChannels)
	}

	if stats.ActiveChannels != 1 {
		t.Errorf("expected 1 active channel, got %d", stats.ActiveChannels)
	}

	if stats.PausedChannels != 1 {
		t.Errorf("expected 1 paused channel, got %d", stats.PausedChannels)
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")
	m.GetOrCreateChannel("slack", "user2", "User 2")

	m.Close()

	stats := m.GetStats()
	if stats.TotalChannels != 0 {
		t.Errorf("expected 0 channels after close, got %d", stats.TotalChannels)
	}
}

func TestManager_SetChannelConfig(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")

	newCfg := &ChannelConfig{
		ID:            "new_id",
		MaxConcurrent: 10,
		Timeout:       10 * time.Minute,
		RateLimit:     100,
	}

	err := m.SetChannelConfig("feishu", "user1", newCfg)
	if err != nil {
		t.Fatalf("SetChannelConfig failed: %v", err)
	}

	ch, _ := m.GetChannel("feishu", "user1")
	if ch.Config.MaxConcurrent != 10 {
		t.Errorf("expected MaxConcurrent=10, got %d", ch.Config.MaxConcurrent)
	}
}

func TestManager_SetRateLimit(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")

	err := m.SetRateLimit("feishu", "user1", 30)
	if err != nil {
		t.Fatalf("SetRateLimit failed: %v", err)
	}

	ch, _ := m.GetChannel("feishu", "user1")
	if ch.Config.RateLimit != 30 {
		t.Errorf("expected RateLimit=30, got %d", ch.Config.RateLimit)
	}
}

func TestManager_SetTimeout(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")

	err := m.SetTimeout("feishu", "user1", 2*time.Minute)
	if err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}

	ch, _ := m.GetChannel("feishu", "user1")
	if ch.Config.Timeout != 2*time.Minute {
		t.Errorf("expected Timeout=2m, got %v", ch.Config.Timeout)
	}
}

func TestManager_SetMaxConcurrent(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")

	err := m.SetMaxConcurrent("feishu", "user1", 5)
	if err != nil {
		t.Fatalf("SetMaxConcurrent failed: %v", err)
	}

	ch, _ := m.GetChannel("feishu", "user1")
	if ch.Config.MaxConcurrent != 5 {
		t.Errorf("expected MaxConcurrent=5, got %d", ch.Config.MaxConcurrent)
	}
}

func TestManager_BroadcastState(t *testing.T) {
	m := NewManager(5)

	m.GetOrCreateChannel("feishu", "user1", "User 1")
	m.GetOrCreateChannel("slack", "user2", "User 2")
	m.GetOrCreateChannel("telegram", "user3", "User 3")

	m.BroadcastState(ChannelPaused)

	stats := m.GetStats()
	if stats.PausedChannels != 3 {
		t.Errorf("expected 3 paused channels, got %d", stats.PausedChannels)
	}
}

func TestChannel_GetStats(t *testing.T) {
	cfg := &ChannelConfig{
		ID:            "test",
		Name:          "Test Channel",
		Platform:      "feishu",
		MaxConcurrent: 5,
		RateLimit:     60,
	}

	ch := NewChannel(cfg)
	ch.RecordRequest()
	ch.RecordRequest()

	stats := ch.GetStats()

	if stats.ID != "test" {
		t.Errorf("expected ID 'test', got %s", stats.ID)
	}

	if stats.RequestCount != 2 {
		t.Errorf("expected 2 requests, got %d", stats.RequestCount)
	}

	if stats.MaxConcurrent != 5 {
		t.Errorf("expected MaxConcurrent=5, got %d", stats.MaxConcurrent)
	}
}

func TestChannel_RateLimitReset(t *testing.T) {
	cfg := &ChannelConfig{
		ID:        "test",
		RateLimit: 2,
	}
	ch := NewChannel(cfg)

	// Hit rate limit
	ch.RecordRequest()
	ch.RecordRequest()

	if ch.CanAccept() {
		t.Error("expected rate limited")
	}

	// Simulate time passing (reset counter)
	ch.mu.Lock()
	ch.lastReqTime = time.Now().Add(-2 * time.Minute)
	ch.mu.Unlock()

	// Should accept again
	if !ch.CanAccept() {
		t.Error("expected to accept after reset")
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    ChannelState
		expected string
	}{
		{ChannelActive, "active"},
		{ChannelPaused, "paused"},
		{ChannelClosed, "closed"},
		{ChannelState(99), "unknown"},
	}

	for _, test := range tests {
		result := StateString(test.state)
		if result != test.expected {
			t.Errorf("StateString(%v) = %s, expected %s", test.state, result, test.expected)
		}
	}
}

func TestManager_Acquire_RateLimited(t *testing.T) {
	m := NewManager(5)

	cfg := &ChannelConfig{
		ID:        "test",
		RateLimit: 1,
	}
	ch := NewChannel(cfg)
	m.channels["feishu:user1"] = ch

	// First request should succeed
	_, cancel, err := m.Acquire("feishu", "user1")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	cancel()
	m.Release()

	// Second should fail (rate limited)
	_, _, err = m.Acquire("feishu", "user1")
	if err == nil {
		t.Error("expected rate limit error")
	}
}
