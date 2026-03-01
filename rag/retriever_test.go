package rag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetriever_New(t *testing.T) {
	tmpDir := t.TempDir()

	r, err := NewRetriever(tmpDir)
	if err != nil {
		t.Fatalf("failed to create retriever: %v", err)
	}
	defer r.Close()

	if r.vectorStore == nil {
		t.Error("expected vector store")
	}

	if r.memoryMgr == nil {
		t.Error("expected memory manager")
	}

	if r.SearchTopK != 5 {
		t.Errorf("expected TopK=5, got %d", r.SearchTopK)
	}
}

func TestRetriever_AddKnowledge(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	doc, err := r.AddKnowledge("Go is a programming language", map[string]string{
		"category": "programming",
	})
	if err != nil {
		t.Fatalf("failed to add knowledge: %v", err)
	}

	if doc == nil {
		t.Fatal("expected document")
	}

	stats := r.Stats()
	if stats.DocumentCount != 1 {
		t.Errorf("expected 1 document, got %d", stats.DocumentCount)
	}

	if stats.WorkingCount != 1 {
		t.Errorf("expected 1 working memory entry, got %d", stats.WorkingCount)
	}
}

func TestRetriever_Retrieve(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	// Add knowledge
	r.AddKnowledge("Go is a statically typed, compiled programming language", map[string]string{
		"topic": "go",
	})
	r.AddKnowledge("Python is an interpreted, high-level programming language", map[string]string{
		"topic": "python",
	})
	r.AddKnowledge("Java is a class-based, object-oriented programming language", map[string]string{
		"topic": "java",
	})

	// Retrieve for Go query
	docs, working, err := r.Retrieve("Go language", 5)
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}

	if len(docs) == 0 {
		t.Error("expected documents")
	}

	_ = working
}

func TestRetriever_RetrieveWithMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.AddKnowledge("Go tutorial basics", map[string]string{"series": "go", "level": "beginner"})
	r.AddKnowledge("Go tutorial advanced", map[string]string{"series": "go", "level": "advanced"})
	r.AddKnowledge("Python tutorial basics", map[string]string{"series": "python", "level": "beginner"})

	// Filter by series=go
	docs, err := r.RetrieveWithMetadata("tutorial", 5, map[string]string{"series": "go"})
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("expected 2 docs for Go series, got %d", len(docs))
	}

	// Filter by series=go AND level=beginner
	docs, err = r.RetrieveWithMetadata("tutorial", 5, map[string]string{"series": "go", "level": "beginner"})
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}

	if len(docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(docs))
	}
}

func TestRetriever_GetContext(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.AddKnowledge("The capital of France is Paris", map[string]string{"category": "geography"})
	r.AddKnowledge("The capital of Germany is Berlin", map[string]string{"category": "geography"})
	r.AddKnowledge("The capital of Japan is Tokyo", map[string]string{"category": "geography"})

	ctx, err := r.GetContext("France capital", 1000)
	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}

	if !strings.Contains(ctx, "Paris") {
		t.Error("expected context to contain Paris")
	}

	if !strings.Contains(ctx, "Relevant Knowledge") {
		t.Error("expected context header")
	}
}

func TestRetriever_GetContext_Truncation(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	// Add long content
	longContent := strings.Repeat("This is a long sentence. ", 100)
	r.AddKnowledge(longContent, nil)

	ctx, err := r.GetContext("test", 500)
	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}

	if len(ctx) > 600 { // 500 + some buffer
		t.Errorf("context too long: %d chars", len(ctx))
	}
}

func TestRetriever_SetMinScore(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.SetMinScore(0.5)
	if r.MinScore != 0.5 {
		t.Errorf("expected MinScore=0.5, got %f", r.MinScore)
	}
}

func TestRetriever_SetTopK(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.SetTopK(10)
	if r.SearchTopK != 10 {
		t.Errorf("expected TopK=10, got %d", r.SearchTopK)
	}
}

func TestRetriever_Stats(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.AddKnowledge("doc1", nil)
	r.AddKnowledge("doc2", nil)
	r.AddKnowledge("doc3", nil)

	stats := r.Stats()

	if stats.DocumentCount != 3 {
		t.Errorf("expected 3 documents, got %d", stats.DocumentCount)
	}

	if stats.WorkingCount != 3 {
		t.Errorf("expected 3 working entries, got %d", stats.WorkingCount)
	}
}

func TestRetriever_Clear(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.AddKnowledge("doc1", nil)
	r.AddKnowledge("doc2", nil)

	err := r.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	stats := r.Stats()
	if stats.DocumentCount != 0 {
		t.Errorf("expected 0 documents, got %d", stats.DocumentCount)
	}

	if stats.WorkingCount != 0 {
		t.Errorf("expected 0 working entries, got %d", stats.WorkingCount)
	}
}

func TestRetriever_ImportDocuments(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	contents := []string{"Imported doc 1", "Imported doc 2", "Imported doc 3"}
	sources := []string{"src1", "src2", "src3"}

	count, err := r.ImportDocuments(contents, sources)
	if err != nil {
		t.Fatalf("ImportDocuments failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 imported, got %d", count)
	}

	stats := r.Stats()
	if stats.DocumentCount != 3 {
		t.Errorf("expected 3 documents, got %d", stats.DocumentCount)
	}
}

func TestRetriever_ExportDocuments(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.AddKnowledge("Export test 1", nil)
	r.AddKnowledge("Export test 2", nil)

	docs := r.ExportDocuments()

	if len(docs) != 2 {
		t.Errorf("expected 2 docs, got %d", len(docs))
	}
}

func TestRetriever_DeleteDocument(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	doc, _ := r.AddKnowledge("To be deleted", nil)

	err := r.DeleteDocument(doc.ID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	stats := r.Stats()
	if stats.DocumentCount != 0 {
		t.Errorf("expected 0 documents, got %d", stats.DocumentCount)
	}
}

func TestRetriever_GetDocument(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	doc, _ := r.AddKnowledge("Get test", nil)

	retrieved, err := r.GetDocument(doc.ID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	if retrieved.Content != "Get test" {
		t.Errorf("expected 'Get test', got %s", retrieved.Content)
	}
}

func TestRetriever_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and add
	r1, _ := NewRetriever(tmpDir)
	r1.AddKnowledge("Persistent knowledge", map[string]string{"key": "value"})
	r1.Close()

	// Reload
	r2, err := NewRetriever(tmpDir)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	defer r2.Close()

	stats := r2.Stats()
	if stats.DocumentCount == 0 {
		t.Error("expected documents to persist")
	}
}

func TestRetriever_FileCreation(t *testing.T) {
	tmpDir := t.TempDir()

	r, _ := NewRetriever(tmpDir)
	defer r.Close()

	r.AddKnowledge("test", nil)

	// Check RAG documents file exists
	ragFile := filepath.Join(tmpDir, "rag_documents.json")
	if _, err := os.Stat(ragFile); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", ragFile)
	}

	// Force memory save to create long_term_memories.json
	r.memoryMgr.Save()

	// Long-term memories file is created on first save with data
	// Trigger compression to create long-term memories
	r.memoryMgr.Compressor.MaxEntriesBeforeCompress = 1
	r.memoryMgr.AddMessage("trigger compression", nil)

	ltmFile := filepath.Join(tmpDir, "long_term_memories.json")
	if _, err := os.Stat(ltmFile); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", ltmFile)
	}
}
