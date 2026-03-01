package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompressor_CompressAndMove(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "ltm.json")

	lm, _ := NewLongTermMemory(storePath, 100)
	wm := NewWorkingMemory(10, 10000)

	// Add entries to working memory
	for i := 0; i < 5; i++ {
		wm.Add("message "+string(rune('A'+i)), nil)
	}

	compressor := NewCompressor(3, lm)

	// Should trigger compression (5 >= 3)
	count, err := compressor.CompressAndMove(wm)
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	if count != 5 {
		t.Errorf("expected 5 entries compressed, got %d", count)
	}

	if wm.Len() != 0 {
		t.Errorf("expected working memory cleared, got %d entries", wm.Len())
	}

	if lm.Len() != 1 {
		t.Errorf("expected 1 long-term memory, got %d", lm.Len())
	}
}

func TestCompressor_NoCompressBelowThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "ltm.json")

	lm, _ := NewLongTermMemory(storePath, 100)
	wm := NewWorkingMemory(10, 10000)

	// Add fewer entries than threshold
	wm.Add("message 1", nil)
	wm.Add("message 2", nil)

	compressor := NewCompressor(5, lm)

	// Should NOT trigger compression (2 < 5)
	count, err := compressor.CompressAndMove(wm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 entries compressed, got %d", count)
	}

	if wm.Len() != 2 {
		t.Errorf("expected 2 entries in working memory, got %d", wm.Len())
	}
}

func TestCompressor_CustomCompressFunc(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "ltm.json")

	lm, _ := NewLongTermMemory(storePath, 100)
	wm := NewWorkingMemory(10, 10000)

	wm.Add("test1", map[string]string{"category": "test"})
	wm.Add("test2", map[string]string{"category": "test"})

	compressor := NewCompressor(1, lm)
	compressor.SetCompressFunc(func(entries []*MemoryEntry) (string, error) {
		return "custom summary: " + string(rune(len(entries)+'0')), nil
	})

	count, err := compressor.CompressAndMove(wm)
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 entries compressed, got %d", count)
	}

	// Check the custom summary was used
	entries := lm.GetAll()
	if len(entries) > 0 && !strings.HasPrefix(entries[0].Summary, "custom summary") {
		t.Errorf("expected custom summary, got %s", entries[0].Summary)
	}
}

func TestDefaultCompress(t *testing.T) {
	compressor := NewCompressor(5, nil)

	entries := []*MemoryEntry{
		{Content: "First message", Metadata: map[string]string{"category": "chat"}},
		{Content: "Second message", Metadata: map[string]string{"category": "chat"}},
		{Content: "Decision: use option A", Metadata: map[string]string{"category": "decision"}},
	}

	summary, err := compressor.defaultCompress(entries)
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	if !strings.Contains(summary, "3 entries") {
		t.Error("expected summary to mention entry count")
	}

	if !strings.Contains(summary, "[chat]") {
		t.Error("expected summary to contain chat category")
	}

	if !strings.Contains(summary, "[decision]") {
		t.Error("expected summary to contain decision category")
	}
}

func TestSummarizer_SummarizeTurn(t *testing.T) {
	summarizer := NewSummarizer(100, true)

	userMsg := "What is the best way to learn Go?"
	assistantMsg := "The best way to learn Go is to practice by building projects."

	summary := summarizer.SummarizeTurn(userMsg, assistantMsg)

	if !strings.Contains(summary, "User:") {
		t.Error("expected summary to contain User")
	}

	if !strings.Contains(summary, "Assistant:") {
		t.Error("expected summary to contain Assistant")
	}

	if !strings.Contains(summary, "learn Go") {
		t.Error("expected summary to contain key content")
	}
}

func TestSummarizer_SummarizeTurn_Truncation(t *testing.T) {
	summarizer := NewSummarizer(50, true)

	longMsg := strings.Repeat("This is a very long message. ", 20)

	summary := summarizer.SummarizeTurn("Short", longMsg)

	if len(summary) > 200 {
		t.Errorf("summary too long: %d chars", len(summary))
	}
}

func TestSummarizer_ExtractKeyPoints(t *testing.T) {
	summarizer := NewSummarizer(100, true)

	entries := []*MemoryEntry{
		{Content: "Normal conversation message"},
		{Content: "TODO: Implement the compression feature"},
		{Content: "Decision: We will use Go for the backend"},
		{Content: "Regular chat message"},
	}

	keyPoints := summarizer.ExtractKeyPoints(entries)

	// Should extract TODO and Decision entries
	if len(keyPoints) < 2 {
		t.Errorf("expected at least 2 key points, got %d", len(keyPoints))
	}
}

func TestSummarizer_IsImportant(t *testing.T) {
	summarizer := NewSummarizer(100, true)

	tests := []struct {
		content  string
		expected bool
	}{
		{"TODO: fix the bug", true},
		{"Decision: use option B", true},
		{"We decided to proceed", true},
		{"Changed the implementation", true},
		{"Regular chat message", false},
		{"Hello, how are you?", false},
	}

	for _, test := range tests {
		result := summarizer.isImportant(test.content)
		if result != test.expected {
			t.Errorf("isImportant(%q) = %v, expected %v", test.content, result, test.expected)
		}
	}
}

func TestMemoryManager_AddMessage(t *testing.T) {
	tmpDir := t.TempDir()

	mm, err := NewMemoryManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}
	defer mm.Close()

	// Manually set a lower threshold for testing
	mm.Compressor.MaxEntriesBeforeCompress = 5

	// Add messages
	for i := 0; i < 10; i++ {
		entry, evicted, err := mm.AddMessage("Test message "+string(rune('A'+i)), nil)
		if err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}

		if entry == nil {
			t.Error("expected entry, got nil")
		}

		_ = evicted
	}

	// Should have triggered compression
	if mm.LongTerm.Len() == 0 {
		t.Error("expected long-term memories after compression")
	}
}

func TestMemoryManager_GetContext(t *testing.T) {
	tmpDir := t.TempDir()

	mm, err := NewMemoryManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}
	defer mm.Close()

	// Add messages with metadata
	mm.AddMessage("User question about Go", map[string]string{"topic": "go"})
	mm.AddMessage("Assistant answer about Go", map[string]string{"topic": "go"})
	mm.AddMessage("User question about Python", map[string]string{"topic": "python"})

	// Get context with query
	working, longTerm := mm.GetContext("Go", 10, 10)

	if len(working) == 0 {
		t.Error("expected working memory results")
	}

	_ = longTerm
}

func TestMemoryManager_Clear(t *testing.T) {
	tmpDir := t.TempDir()

	mm, err := NewMemoryManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}
	defer mm.Close()

	mm.AddMessage("Test message 1", nil)
	mm.AddMessage("Test message 2", nil)

	err = mm.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if mm.Working.Len() != 0 {
		t.Errorf("expected 0 working memories, got %d", mm.Working.Len())
	}

	if mm.LongTerm.Len() != 0 {
		t.Errorf("expected 0 long-term memories, got %d", mm.LongTerm.Len())
	}
}

func TestMemoryManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and add memories
	mm1, _ := NewMemoryManager(tmpDir)
	mm1.AddMessage("Persistent message", nil)
	mm1.LongTerm.Add("Direct long-term", "", nil)
	mm1.Close()

	// Reload
	mm2, err := NewMemoryManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to reload memory manager: %v", err)
	}
	defer mm2.Close()

	// Should have loaded previous memories
	if mm2.LongTerm.Len() == 0 {
		t.Error("expected long-term memories to persist")
	}
}

func TestMemoryManager_SessionPath(t *testing.T) {
	tmpDir := t.TempDir()

	mm, _ := NewMemoryManager(tmpDir)
	defer mm.Close()

	// Add a message to trigger file creation
	mm.AddMessage("Test message", nil)

	// Force save to ensure file is written
	mm.Save()

	// Check that the long-term memory file was created
	ltmPath := filepath.Join(tmpDir, "long_term_memories.json")
	if _, err := os.Stat(ltmPath); os.IsNotExist(err) {
		t.Error("expected long-term memory file to exist")
	}
}
