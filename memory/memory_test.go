package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkingMemory_Add(t *testing.T) {
	wm := NewWorkingMemory(5, 1000)

	entry, evicted := wm.Add("Hello, this is a test message", nil)

	if entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if entry.Type != MemoryShortTerm {
		t.Errorf("expected type short_term, got %v", entry.Type)
	}

	if len(evicted) != 0 {
		t.Errorf("expected no evictions, got %d", len(evicted))
	}

	if wm.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", wm.Len())
	}
}

func TestWorkingMemory_MaxEntries(t *testing.T) {
	wm := NewWorkingMemory(3, 10000)

	wm.Add("message 1", nil)
	wm.Add("message 2", nil)
	wm.Add("message 3", nil)
	wm.Add("message 4", nil) // Should evict message 1

	if wm.Len() != 3 {
		t.Errorf("expected 3 entries after eviction, got %d", wm.Len())
	}

	entries := wm.GetAll()
	if entries[0].Content != "message 2" {
		t.Errorf("expected oldest to be message 2, got %s", entries[0].Content)
	}
}

func TestWorkingMemory_Get(t *testing.T) {
	wm := NewWorkingMemory(10, 10000)

	wm.Add("msg1", nil)
	wm.Add("msg2", nil)
	wm.Add("msg3", nil)

	entries := wm.Get(2)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Should return most recent entries
	if entries[0].Content != "msg2" {
		t.Errorf("expected msg2, got %s", entries[0].Content)
	}
	if entries[1].Content != "msg3" {
		t.Errorf("expected msg3, got %s", entries[1].Content)
	}
}

func TestWorkingMemory_Clear(t *testing.T) {
	wm := NewWorkingMemory(10, 10000)

	wm.Add("msg1", nil)
	wm.Add("msg2", nil)

	wm.Clear()

	if wm.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", wm.Len())
	}
}

func TestLongTermMemory_Add(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, err := NewLongTermMemory(storePath, 100)
	if err != nil {
		t.Fatalf("failed to create long-term memory: %v", err)
	}

	entry, err := lm.Add("This is important information", "Important info", map[string]string{
		"source": "test",
	})
	if err != nil {
		t.Fatalf("failed to add memory: %v", err)
	}

	if entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if entry.Type != MemoryLongTerm {
		t.Errorf("expected type long_term, got %v", entry.Type)
	}

	if lm.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", lm.Len())
	}
}

func TestLongTermMemory_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	// Create and add memory
	lm1, _ := NewLongTermMemory(storePath, 100)
	lm1.Add("Persistent memory", "Summary", nil)

	// Create new instance and load
	lm2, err := NewLongTermMemory(storePath, 100)
	if err != nil {
		t.Fatalf("failed to load long-term memory: %v", err)
	}

	if lm2.Len() != 1 {
		t.Errorf("expected 1 entry after reload, got %d", lm2.Len())
	}

	entries := lm2.GetAll()
	if len(entries) > 0 && entries[0].Content != "Persistent memory" {
		t.Errorf("expected 'Persistent memory', got %s", entries[0].Content)
	}
}

func TestLongTermMemory_Search(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, _ := NewLongTermMemory(storePath, 100)

	lm.Add("The quick brown fox jumps over the lazy dog", "Fox and dog", nil)
	lm.Add("To be or not to be, that is the question", "Hamlet quote", nil)
	lm.Add("The answer to life, the universe and everything is 42", "Life answer", nil)

	results := lm.Search("the", 10)
	if len(results) == 0 {
		t.Error("expected search results, got none")
	}
}

func TestLongTermMemory_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, _ := NewLongTermMemory(storePath, 100)

	entry, _ := lm.Add("Memory to delete", "", nil)

	if lm.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", lm.Len())
	}

	err := lm.Delete(entry.ID)
	if err != nil {
		t.Fatalf("failed to delete memory: %v", err)
	}

	if lm.Len() != 0 {
		t.Errorf("expected 0 entries after delete, got %d", lm.Len())
	}
}

func TestLongTermMemory_MaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, _ := NewLongTermMemory(storePath, 3)

	lm.Add("msg1", "", nil)
	lm.Add("msg2", "", nil)
	lm.Add("msg3", "", nil)
	lm.Add("msg4", "", nil) // Should prune

	if lm.Len() != 3 {
		t.Errorf("expected 3 entries after pruning, got %d", lm.Len())
	}
}

func TestLongTermMemory_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, _ := NewLongTermMemory(storePath, 100)

	lm.Add("memory1", "", nil)
	lm.Add("memory2", "", nil)

	err := lm.Clear()
	if err != nil {
		t.Fatalf("failed to clear memories: %v", err)
	}

	if lm.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", lm.Len())
	}
}

func TestWorkingMemory_Metadata(t *testing.T) {
	wm := NewWorkingMemory(10, 10000)

	metadata := map[string]string{
		"source":   "user",
		"category": "important",
	}

	entry, _ := wm.Add("Important content", metadata)

	if entry.Metadata["source"] != "user" {
		t.Errorf("expected source 'user', got %s", entry.Metadata["source"])
	}

	if entry.Metadata["category"] != "important" {
		t.Errorf("expected category 'important', got %s", entry.Metadata["category"])
	}
}

func TestMemoryEntry_AccessTracking(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, _ := NewLongTermMemory(storePath, 100)

	entry, _ := lm.Add("Test content", "", nil)
	initialAccess := entry.AccessCount

	// Access the entry via Get
	_, err := lm.Get(entry.ID)
	if err != nil {
		t.Fatalf("failed to get memory: %v", err)
	}

	// Reload and check
	reloaded, _ := lm.Get(entry.ID)
	if reloaded.AccessCount <= initialAccess {
		t.Error("expected access count to increase")
	}
}

func TestFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "memories.json")

	lm, _ := NewLongTermMemory(storePath, 100)
	lm.Add("test", "", nil)

	// Check file permissions
	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	// Should be 0644
	mode := info.Mode().Perm()
	if mode != 0644 {
		t.Errorf("expected mode 0644, got %o", mode)
	}
}
