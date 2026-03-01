// Package memory implements short-term and long-term memory management
// with automatic compression and persistence for AI agent conversations.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryType distinguishes between short-term and long-term memory.
type MemoryType string

const (
	MemoryShortTerm MemoryType = "short_term"
	MemoryLongTerm  MemoryType = "long_term"
)

// MemoryEntry represents a single memory item.
type MemoryEntry struct {
	ID        string            `json:"id"`
	Type      MemoryType        `json:"type"`
	Content   string            `json:"content"`
	Summary   string            `json:"summary,omitempty"` // for compressed long-term memories
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	AccessedAt time.Time        `json:"accessed_at"`
	AccessCount int             `json:"access_count"`
}

// WorkingMemory holds the short-term, immediately accessible memories.
type WorkingMemory struct {
	mu          sync.RWMutex
	entries     []*MemoryEntry
	MaxEntries  int
	MaxTokens   int // approximate token limit
	CurrentTokens int
}

// NewWorkingMemory creates a new working memory with default limits.
func NewWorkingMemory(maxEntries int, maxTokens int) *WorkingMemory {
	return &WorkingMemory{
		entries:    make([]*MemoryEntry, 0),
		MaxEntries: maxEntries,
		MaxTokens:  maxTokens,
	}
}

// Add adds a new entry to working memory.
// Returns the added entry and any entries that were evicted.
func (wm *WorkingMemory) Add(content string, metadata map[string]string) (*MemoryEntry, []*MemoryEntry) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	entry := &MemoryEntry{
		ID:         fmt.Sprintf("wm_%d_%d", time.Now().UnixNano(), len(wm.entries)),
		Type:       MemoryShortTerm,
		Content:    content,
		Metadata:   metadata,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	wm.entries = append(wm.entries, entry)
	wm.CurrentTokens += estimateTokens(content)

	// Evict oldest entries if over limit
	var evicted []*MemoryEntry
	for len(wm.entries) > wm.MaxEntries || wm.CurrentTokens > wm.MaxTokens {
		if len(wm.entries) == 0 {
			break
		}
		oldest := wm.entries[0]
		wm.entries = wm.entries[1:]
		wm.CurrentTokens -= estimateTokens(oldest.Content)
		evicted = append(evicted, oldest)
	}

	return entry, evicted
}

// Get returns entries from working memory.
func (wm *WorkingMemory) Get(limit int) []*MemoryEntry {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if limit <= 0 || limit > len(wm.entries) {
		limit = len(wm.entries)
	}

	// Return most recent entries
	start := len(wm.entries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*MemoryEntry, limit)
	for i := 0; i < limit; i++ {
		result[i] = wm.entries[start+i]
		result[i].AccessCount++
		result[i].AccessedAt = time.Now()
	}
	return result
}

// GetAll returns all entries without updating access counts.
func (wm *WorkingMemory) GetAll() []*MemoryEntry {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	result := make([]*MemoryEntry, len(wm.entries))
	copy(result, wm.entries)
	return result
}

// Clear removes all entries from working memory.
func (wm *WorkingMemory) Clear() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.entries = make([]*MemoryEntry, 0)
	wm.CurrentTokens = 0
}

// Len returns the number of entries.
func (wm *WorkingMemory) Len() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.entries)
}

// estimateTokens provides a rough token estimate (1 token ≈ 4 chars for English).
func estimateTokens(s string) int {
	return len(s) / 4
}

// LongTermMemory persists memories to disk and supports retrieval.
type LongTermMemory struct {
	mu         sync.RWMutex
	entries    map[string]*MemoryEntry // id -> entry
	storePath  string
	MaxEntries int
}

// NewLongTermMemory creates a new long-term memory with persistence.
func NewLongTermMemory(storePath string, maxEntries int) (*LongTermMemory, error) {
	lm := &LongTermMemory{
		entries:    make(map[string]*MemoryEntry),
		storePath:  storePath,
		MaxEntries: maxEntries,
	}

	if err := lm.load(); err != nil {
		return nil, err
	}

	return lm, nil
}

// Add adds a new long-term memory entry.
func (lm *LongTermMemory) Add(content string, summary string, metadata map[string]string) (*MemoryEntry, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	entry := &MemoryEntry{
		ID:         fmt.Sprintf("ltm_%d_%s", time.Now().UnixNano(), entryID(5)),
		Type:       MemoryLongTerm,
		Content:    content,
		Summary:    summary,
		Metadata:   metadata,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	lm.entries[entry.ID] = entry

	// Prune if over limit
	if len(lm.entries) > lm.MaxEntries {
		lm.pruneOldest()
	}

	if err := lm.save(); err != nil {
		return nil, err
	}

	return entry, nil
}

// AddCompressed adds a compressed summary as long-term memory.
func (lm *LongTermMemory) AddCompressed(summary string, originalEntries []*MemoryEntry, metadata map[string]string) (*MemoryEntry, error) {
	// Combine original contents for archival
	var content string
	for _, e := range originalEntries {
		content += e.Content + "\n\n"
	}

	return lm.Add(summary, summary, metadata)
}

// Get retrieves a specific memory by ID.
func (lm *LongTermMemory) Get(id string) (*MemoryEntry, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	entry, ok := lm.entries[id]
	if !ok {
		return nil, fmt.Errorf("memory %s not found", id)
	}

	// Update access stats
	entry.AccessCount++
	entry.AccessedAt = time.Now()

	return entry, nil
}

// Search finds memories containing the query text.
func (lm *LongTermMemory) Search(query string, limit int) []*MemoryEntry {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var results []*MemoryEntry
	for _, entry := range lm.entries {
		if containsIgnoreCase(entry.Content, query) ||
			containsIgnoreCase(entry.Summary, query) {
			results = append(results, entry)
			entry.AccessCount++
			entry.AccessedAt = time.Now()
		}
		if len(results) >= limit {
			break
		}
	}

	return results
}

// GetAll returns all long-term memories.
func (lm *LongTermMemory) GetAll() []*MemoryEntry {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make([]*MemoryEntry, 0, len(lm.entries))
	for _, entry := range lm.entries {
		result = append(result, entry)
	}
	return result
}

// Delete removes a memory by ID.
func (lm *LongTermMemory) Delete(id string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	delete(lm.entries, id)
	return lm.save()
}

// Clear removes all memories.
func (lm *LongTermMemory) Clear() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.entries = make(map[string]*MemoryEntry)
	return lm.save()
}

// Len returns the number of entries.
func (lm *LongTermMemory) Len() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return len(lm.entries)
}

func (lm *LongTermMemory) pruneOldest() {
	// Find and remove the oldest entry by UpdatedAt
	var oldestID string
	var oldestTime time.Time

	for id, entry := range lm.entries {
		if oldestID == "" || entry.UpdatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = entry.UpdatedAt
		}
	}

	if oldestID != "" {
		delete(lm.entries, oldestID)
	}
}

func (lm *LongTermMemory) save() error {
	if lm.storePath == "" {
		return nil
	}

	data, err := json.MarshalIndent(lm.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal memories: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(lm.storePath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(lm.storePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (lm *LongTermMemory) load() error {
	if lm.storePath == "" {
		return nil
	}

	data, err := os.ReadFile(lm.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, not an error
		}
		return fmt.Errorf("failed to read file: %w", err)
	}

	var entries map[string]*MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal memories: %w", err)
	}

	lm.entries = entries
	return nil
}

// entryID generates a short random ID suffix.
func entryID(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || containsFolded(s, substr))
}

func containsFolded(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	// Simple ASCII lowercase
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
