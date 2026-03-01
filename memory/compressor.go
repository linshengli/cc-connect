package memory

import (
	"fmt"
	"strings"
	"time"
)

// Compressor handles compression of working memories into long-term storage.
type Compressor struct {
	// CompressFunc is called to compress multiple entries into a summary.
	// If nil, a default concatenation strategy is used.
	CompressFunc func(entries []*MemoryEntry) (string, error)

	// MaxEntriesBeforeCompress is the threshold that triggers compression.
	MaxEntriesBeforeCompress int

	// CompressToLongTerm is the destination for compressed memories.
	CompressToLongTerm *LongTermMemory
}

// NewCompressor creates a new memory compressor.
func NewCompressor(maxEntries int, longTerm *LongTermMemory) *Compressor {
	return &Compressor{
		MaxEntriesBeforeCompress: maxEntries,
		CompressToLongTerm:       longTerm,
	}
}

// CompressAndMove compresses entries from working memory and moves them to long-term storage.
// Returns the number of entries compressed and any error.
func (c *Compressor) CompressAndMove(wm *WorkingMemory) (int, error) {
	entries := wm.GetAll()

	if len(entries) < c.MaxEntriesBeforeCompress {
		return 0, nil // Not enough entries to compress
	}

	// Compress the entries
	summary, err := c.compress(entries)
	if err != nil {
		return 0, err
	}

	// Create metadata with compression info
	metadata := map[string]string{
		"compressed_from":   "working_memory",
		"original_count":    fmt.Sprintf("%d", len(entries)),
		"compressed_at":     time.Now().Format(time.RFC3339),
		"compression_ratio": fmt.Sprintf("%.2f", float64(len(entries))),
	}

	// Add to long-term memory
	_, err = c.CompressToLongTerm.Add(summary, summary, metadata)
	if err != nil {
		return 0, err
	}

	// Clear working memory entries that were compressed
	wm.Clear()

	return len(entries), nil
}

// CompressAndMoveSelected compresses specific entries and moves them to long-term storage.
func (c *Compressor) CompressAndMoveSelected(entries []*MemoryEntry) (*MemoryEntry, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	summary, err := c.compress(entries)
	if err != nil {
		return nil, err
	}

	metadata := map[string]string{
		"compressed_from": "selected",
		"original_count":  fmt.Sprintf("%d", len(entries)),
		"compressed_at":   time.Now().Format(time.RFC3339),
	}

	return c.CompressToLongTerm.Add(summary, summary, metadata)
}

// SetCompressFunc sets a custom compression function.
func (c *Compressor) SetCompressFunc(fn func([]*MemoryEntry) (string, error)) {
	c.CompressFunc = fn
}

// compress performs the actual compression using the configured function or default.
func (c *Compressor) compress(entries []*MemoryEntry) (string, error) {
	if c.CompressFunc != nil {
		return c.CompressFunc(entries)
	}
	return c.defaultCompress(entries)
}

// defaultCompress provides a simple concatenation-based compression.
func (c *Compressor) defaultCompress(entries []*MemoryEntry) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}

	// Group by metadata category if available
	categorized := make(map[string][]string)
	for _, entry := range entries {
		category := "misc"
		if cat, ok := entry.Metadata["category"]; ok {
			category = cat
		}
		categorized[category] = append(categorized[category], entry.Content)
	}

	// Build summary
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("=== Conversation Summary (%d entries) ===\n\n", len(entries)))

	for category, contents := range categorized {
		summary.WriteString(fmt.Sprintf("[%s]\n", category))
		for i, content := range contents {
			// Truncate very long entries
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			summary.WriteString(fmt.Sprintf("  %d. %s\n", i+1, content))
		}
		summary.WriteString("\n")
	}

	summary.WriteString(fmt.Sprintf("=== End of Summary (compressed at %s) ===", time.Now().Format("2006-01-02 15:04:05")))

	return summary.String(), nil
}

// Summarizer generates summaries of conversation turns.
type Summarizer struct {
	// MaxSummaryLength limits the output summary length.
	MaxSummaryLength int

	// KeepKeyPoints determines whether to extract key points separately.
	KeepKeyPoints bool
}

// NewSummarizer creates a new summarizer.
func NewSummarizer(maxLen int, keepKeyPoints bool) *Summarizer {
	return &Summarizer{
		MaxSummaryLength: maxLen,
		KeepKeyPoints:    keepKeyPoints,
	}
}

// SummarizeTurn creates a summary of a conversation turn.
func (s *Summarizer) SummarizeTurn(userMsg, assistantMsg string) string {
	var summary strings.Builder

	// Truncate if necessary
	if len(userMsg) > 200 {
		userMsg = userMsg[:200] + "..."
	}
	if len(assistantMsg) > s.MaxSummaryLength {
		assistantMsg = assistantMsg[:s.MaxSummaryLength] + "..."
	}

	summary.WriteString(fmt.Sprintf("User: %s\n", userMsg))
	summary.WriteString(fmt.Sprintf("Assistant: %s", assistantMsg))

	return summary.String()
}

// ExtractKeyPoints extracts key points from a conversation.
func (s *Summarizer) ExtractKeyPoints(entries []*MemoryEntry) []string {
	var keyPoints []string

	for _, entry := range entries {
		// Look for common patterns that indicate important information
		content := entry.Content

		// Check for TODOs, decisions, code changes, etc.
		if s.isImportant(content) {
			keyPoints = append(keyPoints, s.extractPoint(content))
		}
	}

	return keyPoints
}

func (s *Summarizer) isImportant(content string) bool {
	importantMarkers := []string{
		"TODO", "FIXME", "NOTE", "IMPORTANT",
		"decided", "decision", "conclusion",
		"changed", "modified", "added", "removed",
		"key point", "summary", "conclusion",
	}

	lower := strings.ToLower(content)
	for _, marker := range importantMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func (s *Summarizer) extractPoint(content string) string {
	// Simple extraction: take first line or first 100 chars
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		point := strings.TrimSpace(lines[0])
		if len(point) > 100 {
			point = point[:100] + "..."
		}
		return point
	}
	return content
}

// MemoryManager coordinates working memory, long-term memory, and compression.
type MemoryManager struct {
	Working     *WorkingMemory
	LongTerm    *LongTermMemory
	Compressor  *Compressor
	Summarizer  *Summarizer
	sessionPath string
}

// NewMemoryManager creates a new memory manager with all components.
func NewMemoryManager(sessionPath string) (*MemoryManager, error) {
	// Create working memory (limit: 50 entries or ~4000 tokens)
	wm := NewWorkingMemory(50, 4000)

	// Create long-term memory with persistence
	ltmPath := filepathJoin(sessionPath, "long_term_memories.json")
	ltm, err := NewLongTermMemory(ltmPath, 1000)
	if err != nil {
		return nil, err
	}

	// Create compressor (compress after 40 entries)
	compressor := NewCompressor(40, ltm)

	// Create summarizer
	summarizer := NewSummarizer(500, true)

	return &MemoryManager{
		Working:    wm,
		LongTerm:   ltm,
		Compressor: compressor,
		Summarizer: summarizer,
	}, nil
}

// AddMessage adds a message to working memory and triggers compression if needed.
func (mm *MemoryManager) AddMessage(content string, metadata map[string]string) (*MemoryEntry, []*MemoryEntry, error) {
	entry, evicted := mm.Working.Add(content, metadata)

	// Check if compression should be triggered
	if mm.Working.Len() >= mm.Compressor.MaxEntriesBeforeCompress {
		compressed, err := mm.Compressor.CompressAndMove(mm.Working)
		if err != nil {
			return entry, evicted, err
		}
		if compressed > 0 {
			// Log compression event
			fmt.Printf("Memory compression: moved %d entries to long-term storage\n", compressed)
		}
	}

	return entry, evicted, nil
}

// GetContext retrieves relevant context for a query.
// Returns working memory entries + relevant long-term memories.
func (mm *MemoryManager) GetContext(query string, workingLimit int, longTermLimit int) ([]*MemoryEntry, []*MemoryEntry) {
	working := mm.Working.Get(workingLimit)
	longTerm := mm.LongTerm.Search(query, longTermLimit)
	return working, longTerm
}

// GetRecentContext returns recent working memory entries without search.
func (mm *MemoryManager) GetRecentContext(limit int) []*MemoryEntry {
	return mm.Working.Get(limit)
}

// GetAllLongTerm returns all long-term memories.
func (mm *MemoryManager) GetAllLongTerm() []*MemoryEntry {
	return mm.LongTerm.GetAll()
}

// Clear clears all memories.
func (mm *MemoryManager) Clear() error {
	mm.Working.Clear()
	return mm.LongTerm.Clear()
}

// Save persists long-term memories to disk.
func (mm *MemoryManager) Save() error {
	return mm.LongTerm.save()
}

// Close saves and cleans up resources.
func (mm *MemoryManager) Close() error {
	return mm.Save()
}

// filepathJoin is a helper to join paths safely.
func filepathJoin(elem ...string) string {
	var result string
	for i, e := range elem {
		if i == 0 {
			result = e
		} else {
			result = result + "/" + e
		}
	}
	return result
}
