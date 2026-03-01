package rag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVectorStore_AddDocument(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, err := NewVectorStore(storePath)
	if err != nil {
		t.Fatalf("failed to create vector store: %v", err)
	}

	doc, err := vs.AddDocument("This is a test document about Go programming", map[string]string{
		"category": "programming",
		"language": "go",
	})
	if err != nil {
		t.Fatalf("failed to add document: %v", err)
	}

	if doc == nil {
		t.Fatal("expected document, got nil")
	}

	if doc.Content == "" {
		t.Error("expected content")
	}

	if doc.Vector == nil {
		t.Error("expected vector to be computed")
	}

	if vs.Len() != 1 {
		t.Errorf("expected 1 document, got %d", vs.Len())
	}
}

func TestVectorStore_AddDocuments_Batch(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	contents := []string{
		"Introduction to Go programming",
		"Python for data science",
		"JavaScript web development",
	}

	docs, err := vs.AddDocuments(contents, nil)
	if err != nil {
		t.Fatalf("failed to add documents: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("expected 3 documents, got %d", len(docs))
	}

	if vs.Len() != 3 {
		t.Errorf("expected 3 documents in store, got %d", vs.Len())
	}
}

func TestVectorStore_Search(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	// Add documents with distinct topics
	vs.AddDocument("Go is a statically typed, compiled language designed at Google", map[string]string{"topic": "go"})
	vs.AddDocument("Python is an interpreted, high-level programming language", map[string]string{"topic": "python"})
	vs.AddDocument("JavaScript is a scripting language for web development", map[string]string{"topic": "js"})

	// Search for Go-related content
	results := vs.Search("Go language Google", 5)

	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	// First result should be the Go document
	if !strings.Contains(results[0].Content, "Go") {
		t.Errorf("expected first result to be about Go, got: %s", results[0].Content)
	}
}

func TestVectorStore_Search_Relevance(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	// Add documents
	vs.AddDocument("The quick brown fox jumps over the lazy dog", nil)
	vs.AddDocument("Dogs are loyal pets and great companions", nil)
	vs.AddDocument("Cats are independent animals", nil)

	// Search for "fox dog" - should match first document best
	results := vs.Search("fox dog", 5)

	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	// First result should contain both fox and dog concepts
	if !strings.Contains(strings.ToLower(results[0].Content), "fox") &&
		!strings.Contains(strings.ToLower(results[0].Content), "dog") {
		t.Errorf("expected relevant result, got: %s", results[0].Content)
	}
}

func TestVectorStore_SearchWithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	vs.AddDocument("Go tutorial part 1: basics", map[string]string{"series": "go", "part": "1"})
	vs.AddDocument("Go tutorial part 2: advanced", map[string]string{"series": "go", "part": "2"})
	vs.AddDocument("Python tutorial part 1: basics", map[string]string{"series": "python", "part": "1"})

	// Filter by series=go
	results := vs.SearchWithMetadata("tutorial", 5, map[string]string{"series": "go"})

	if len(results) != 2 {
		t.Errorf("expected 2 results for Go series, got %d", len(results))
	}

	// Filter by series=go AND part=1
	results = vs.SearchWithMetadata("tutorial", 5, map[string]string{"series": "go", "part": "1"})

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestVectorStore_GetDocument(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	doc, _ := vs.AddDocument("Test content", nil)

	retrieved, err := vs.GetDocument(doc.ID)
	if err != nil {
		t.Fatalf("failed to get document: %v", err)
	}

	if retrieved.ID != doc.ID {
		t.Errorf("expected ID %s, got %s", doc.ID, retrieved.ID)
	}

	if retrieved.Content != "Test content" {
		t.Errorf("expected 'Test content', got %s", retrieved.Content)
	}
}

func TestVectorStore_GetDocument_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	_, err := vs.GetDocument("non-existent")
	if err == nil {
		t.Error("expected error for non-existent document")
	}
}

func TestVectorStore_DeleteDocument(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	doc, _ := vs.AddDocument("To be deleted", nil)

	if vs.Len() != 1 {
		t.Fatalf("expected 1 document")
	}

	err := vs.DeleteDocument(doc.ID)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	if vs.Len() != 0 {
		t.Errorf("expected 0 documents after delete, got %d", vs.Len())
	}
}

func TestVectorStore_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	vs.AddDocument("doc1", nil)
	vs.AddDocument("doc2", nil)
	vs.AddDocument("doc3", nil)

	err := vs.Clear()
	if err != nil {
		t.Fatalf("failed to clear: %v", err)
	}

	if vs.Len() != 0 {
		t.Errorf("expected 0 documents after clear, got %d", vs.Len())
	}
}

func TestVectorStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	// Create and add documents
	vs1, _ := NewVectorStore(storePath)
	vs1.AddDocument("Persistent document 1", map[string]string{"key": "value1"})
	vs1.AddDocument("Persistent document 2", map[string]string{"key": "value2"})

	// Reload
	vs2, err := NewVectorStore(storePath)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	if vs2.Len() != 2 {
		t.Errorf("expected 2 documents after reload, got %d", vs2.Len())
	}

	// Search should still work
	results := vs2.Search("Persistent", 5)
	if len(results) != 2 {
		t.Errorf("expected 2 search results, got %d", len(results))
	}
}

func TestVectorStore_EmptySearch(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	results := vs.Search("anything", 5)
	if results != nil {
		t.Error("expected nil results for empty store")
	}
}

func TestVectorStore_TopK(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)
	vs.TopK = 2

	// Add more documents than TopK
	for i := 0; i < 10; i++ {
		vs.AddDocument("Document number "+string(rune('A'+i)), nil)
	}

	// Search should return at most TopK results
	results := vs.Search("Document", 0) // 0 means use default TopK

	if len(results) > 2 {
		t.Errorf("expected at most 2 results (TopK), got %d", len(results))
	}
}

func TestVectorStore_MetadataFilter_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)

	vs.AddDocument("Test document", map[string]string{"category": "A"})

	// Filter that doesn't match
	results := vs.SearchWithMetadata("test", 5, map[string]string{"category": "B"})

	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching filter, got %d", len(results))
	}
}

func TestFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")

	vs, _ := NewVectorStore(storePath)
	vs.AddDocument("test", nil)

	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0644 {
		t.Errorf("expected mode 0644, got %o", mode)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected int // expected token count
	}{
		{"Hello, World!", 2},
		{"Go 1.21 is great", 5}, // "go", "1", "21", "is", "great"
		{"   ", 0},
		{"", 0},
	}

	for _, test := range tests {
		tokens := tokenize(test.input)
		if len(tokens) != test.expected {
			t.Errorf("tokenize(%q) = %d tokens, expected %d", test.input, len(tokens), test.expected)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "documents.json")
	vs, _ := NewVectorStore(storePath)

	// Same text should have perfect similarity
	vec1 := vs.computeVector("identical text here")
	vec2 := vs.computeVector("identical text here")

	sim := cosineSimilarity(vec1, vec2)
	if sim < 0.99 {
		t.Errorf("expected high similarity for identical text, got %f", sim)
	}

	// Different text should have lower similarity
	vec3 := vs.computeVector("completely different words")
	sim2 := cosineSimilarity(vec1, vec3)
	if sim2 >= sim {
		t.Errorf("expected lower similarity for different text")
	}

	// Nil vectors
	sim3 := cosineSimilarity(nil, vec1)
	if sim3 != 0 {
		t.Errorf("expected 0 similarity with nil vector")
	}
}
