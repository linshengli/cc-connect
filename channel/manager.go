// Package channel manages multiple concurrent communication channels
// for routing messages between users and AI agents.
package channel

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ChannelState represents the state of a communication channel.
type ChannelState int

const (
	ChannelActive ChannelState = iota
	ChannelPaused
	ChannelClosed
)

// ChannelConfig holds configuration for a channel.
type ChannelConfig struct {
	ID            string
	Name          string
	Platform      string
	UserID        string
	UserName      string
	MaxConcurrent int           // max concurrent requests
	Timeout       time.Duration // request timeout
	RateLimit     int           // requests per minute (0 = unlimited)
}

// Channel represents a single communication channel.
type Channel struct {
	Config      *ChannelConfig
	State       ChannelState
	CreatedAt   time.Time
	UpdatedAt   time.Time
	lastReqTime time.Time
	reqCount    int
	mu          sync.RWMutex
}

// NewChannel creates a new channel.
func NewChannel(cfg *ChannelConfig) *Channel {
	return &Channel{
		Config:    cfg,
		State:     ChannelActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// CanAccept checks if the channel can accept new requests.
func (c *Channel) CanAccept() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.State != ChannelActive {
		return false
	}

	// Check rate limit
	if c.Config.RateLimit > 0 {
		now := time.Now()
		if now.Sub(c.lastReqTime) < time.Minute {
			if c.reqCount >= c.Config.RateLimit {
				return false
			}
		} else {
			// Reset counter
			c.reqCount = 0
		}
	}

	return true
}

// RecordRequest records a request for rate limiting.
func (c *Channel) RecordRequest() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if now.Sub(c.lastReqTime) >= time.Minute {
		c.reqCount = 0
	}
	c.reqCount++
	c.lastReqTime = now
}

// SetState changes the channel state.
func (c *Channel) SetState(state ChannelState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.State = state
	c.UpdatedAt = time.Now()
}

// GetState returns the current state.
func (c *Channel) GetState() ChannelState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.State
}

// GetStats returns channel statistics.
func (c *Channel) GetStats() ChannelStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ChannelStats{
		ID:            c.Config.ID,
		Name:          c.Config.Name,
		Platform:      c.Config.Platform,
		State:         c.State,
		RequestCount:  c.reqCount,
		RateLimit:     c.Config.RateLimit,
		MaxConcurrent: c.Config.MaxConcurrent,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

// ChannelStats holds statistics for a channel.
type ChannelStats struct {
	ID            string
	Name          string
	Platform      string
	State         ChannelState
	RequestCount  int
	RateLimit     int
	MaxConcurrent int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Manager manages multiple channels with concurrency control.
type Manager struct {
	mu           sync.RWMutex
	channels     map[string]*Channel // key = platform:user_id
	sessionIndex map[string]string   // session_key -> channel_key
	maxGlobal    int                 // max global concurrent requests
	currentGlobal int
	stopCh       chan struct{}
}

// NewManager creates a new channel manager.
func NewManager(maxGlobal int) *Manager {
	return &Manager{
		channels:     make(map[string]*Channel),
		sessionIndex: make(map[string]string),
		maxGlobal:    maxGlobal,
		stopCh:       make(chan struct{}),
	}
}

// GetOrCreateChannel gets or creates a channel for a user.
func (m *Manager) GetOrCreateChannel(platform, userID, userName string) *Channel {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := channelKey(platform, userID)

	if ch, ok := m.channels[key]; ok {
		return ch
	}

	cfg := &ChannelConfig{
		ID:            key,
		Name:          userName,
		Platform:      platform,
		UserID:        userID,
		UserName:      userName,
		MaxConcurrent: 3,        // default
		Timeout:       5 * time.Minute,
		RateLimit:     60,       // default: 60 requests/minute
	}

	ch := NewChannel(cfg)
	m.channels[key] = ch
	return ch
}

// GetChannel retrieves a channel by key.
func (m *Manager) GetChannel(platform, userID string) (*Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := channelKey(platform, userID)
	ch, ok := m.channels[key]
	return ch, ok
}

// RegisterSession registers a session to channel mapping.
func (m *Manager) RegisterSession(sessionKey, platform, userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := channelKey(platform, userID)
	m.sessionIndex[sessionKey] = key
}

// GetChannelBySession gets channel for a session.
func (m *Manager) GetChannelBySession(sessionKey string) (*Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key, ok := m.sessionIndex[sessionKey]
	if !ok {
		return nil, false
	}

	ch, ok := m.channels[key]
	return ch, ok
}

// Acquire acquires a slot for processing.
func (m *Manager) Acquire(platform, userID string) (context.Context, context.CancelFunc, error) {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return nil, nil, errors.New("channel not found")
	}

	if !ch.CanAccept() {
		return nil, nil, errors.New("channel rate limited or not active")
	}

	// Check global limit
	m.mu.Lock()
	if m.currentGlobal >= m.maxGlobal && m.maxGlobal > 0 {
		m.mu.Unlock()
		return nil, nil, errors.New("global rate limit reached")
	}
	m.currentGlobal++
	m.mu.Unlock()

	ch.RecordRequest()

	ctx, cancel := context.WithTimeout(context.Background(), ch.Config.Timeout)
	return ctx, cancel, nil
}

// Release releases a processing slot.
func (m *Manager) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentGlobal > 0 {
		m.currentGlobal--
	}
}

// PauseChannel pauses a channel.
func (m *Manager) PauseChannel(platform, userID string) error {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return errors.New("channel not found")
	}
	ch.SetState(ChannelPaused)
	return nil
}

// ResumeChannel resumes a channel.
func (m *Manager) ResumeChannel(platform, userID string) error {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return errors.New("channel not found")
	}
	ch.SetState(ChannelActive)
	return nil
}

// CloseChannel closes a channel.
func (m *Manager) CloseChannel(platform, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := channelKey(platform, userID)
	if _, ok := m.channels[key]; !ok {
		return errors.New("channel not found")
	}

	m.channels[key].SetState(ChannelClosed)
	delete(m.channels, key)

	// Remove from session index
	for sk, ck := range m.sessionIndex {
		if ck == key {
			delete(m.sessionIndex, sk)
		}
	}

	return nil
}

// ListChannels returns all channels.
func (m *Manager) ListChannels() []ChannelStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]ChannelStats, 0, len(m.channels))
	for _, ch := range m.channels {
		stats = append(stats, ch.GetStats())
	}
	return stats
}

// GetStats returns global statistics.
func (m *Manager) GetStats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := 0
	paused := 0
	closed := 0

	for _, ch := range m.channels {
		switch ch.State {
		case ChannelActive:
			active++
		case ChannelPaused:
			paused++
		case ChannelClosed:
			closed++
		}
	}

	return ManagerStats{
		TotalChannels:   len(m.channels),
		ActiveChannels:  active,
		PausedChannels:  paused,
		ClosedChannels:  closed,
		CurrentGlobal:   m.currentGlobal,
		MaxGlobal:       m.maxGlobal,
	}
}

// ManagerStats holds manager statistics.
type ManagerStats struct {
	TotalChannels   int
	ActiveChannels  int
	PausedChannels  int
	ClosedChannels  int
	CurrentGlobal   int
	MaxGlobal       int
}

// Close closes the manager and all channels.
func (m *Manager) Close() {
	close(m.stopCh)

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ch := range m.channels {
		ch.SetState(ChannelClosed)
	}
	m.channels = make(map[string]*Channel)
	m.sessionIndex = make(map[string]string)
}

// SetChannelConfig updates channel configuration.
func (m *Manager) SetChannelConfig(platform, userID string, cfg *ChannelConfig) error {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return errors.New("channel not found")
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.Config = cfg
	ch.UpdatedAt = time.Now()

	return nil
}

// SetRateLimit sets rate limit for a channel.
func (m *Manager) SetRateLimit(platform, userID string, limit int) error {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return errors.New("channel not found")
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.Config.RateLimit = limit
	ch.UpdatedAt = time.Now()

	return nil
}

// SetTimeout sets timeout for a channel.
func (m *Manager) SetTimeout(platform, userID string, timeout time.Duration) error {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return errors.New("channel not found")
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.Config.Timeout = timeout
	ch.UpdatedAt = time.Now()

	return nil
}

// SetMaxConcurrent sets max concurrent requests for a channel.
func (m *Manager) SetMaxConcurrent(platform, userID string, max int) error {
	ch, ok := m.GetChannel(platform, userID)
	if !ok {
		return errors.New("channel not found")
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.Config.MaxConcurrent = max
	ch.UpdatedAt = time.Now()

	return nil
}

// BroadcastState changes state for all channels.
func (m *Manager) BroadcastState(state ChannelState) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ch := range m.channels {
		ch.SetState(state)
	}
}

// channelKey creates a unique key for a channel.
func channelKey(platform, userID string) string {
	return platform + ":" + userID
}

// StateString returns a string representation of the state.
func StateString(state ChannelState) string {
	switch state {
	case ChannelActive:
		return "active"
	case ChannelPaused:
		return "paused"
	case ChannelClosed:
		return "closed"
	default:
		return "unknown"
	}
}
