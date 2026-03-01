package rag

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/cc-connect/memory"
)

// Retriever combines vector search with memory management for RAG.
type Retriever struct {
	mu           sync.RWMutex
	vectorStore  *VectorStore
	memoryMgr    *memory.MemoryManager
	sessionPath  string
	SearchTopK   int
	MinScore     float64 // minimum similarity score threshold
}

// NewRetriever creates a new RAG retriever.
func NewRetriever(sessionPath string) (*Retriever, error) {
	// Create vector store
	vsPath := sessionPath + "/rag_documents.json"
	vs, err := NewVectorStore(vsPath)
	if err != nil {
		return nil, err
	}

	// Create memory manager
	mm, err := memory.NewMemoryManager(sessionPath)
	if err != nil {
		return nil, err
	}

	return &Retriever{
		vectorStore: vs,
		memoryMgr:   mm,
		sessionPath: sessionPath,
		SearchTopK:  5,
		MinScore:    0.1,
	}, nil
}

// AddKnowledge adds a piece of knowledge to the RAG system.
func (r *Retriever) AddKnowledge(content string, metadata map[string]string) (*Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add to vector store for retrieval
	doc, err := r.vectorStore.AddDocument(content, metadata)
	if err != nil {
		return nil, err
	}

	// Also add to working memory for immediate context
	r.memoryMgr.AddMessage(content, metadata)

	return doc, nil
}

// AddKnowledgeFromMemory compresses working memory and adds to RAG.
func (r *Retriever) AddKnowledgeFromMemory() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get working memory entries
	entries := r.memoryMgr.Working.GetAll()
	if len(entries) == 0 {
		return 0, nil
	}

	// Use the compressor to create summaries
	compressed, err := r.memoryMgr.Compressor.CompressAndMove(r.memoryMgr.Working)
	if err != nil {
		return 0, err
	}

	// Add compressed summaries to vector store
	ltmEntries := r.memoryMgr.LongTerm.GetAll()
	for _, entry := range ltmEntries {
		if entry.Type == memory.MemoryLongTerm {
			r.vectorStore.AddDocument(entry.Summary, entry.Metadata)
		}
	}

	return compressed, nil
}

// Retrieve performs RAG retrieval for a query.
func (r *Retriever) Retrieve(query string, k int) ([]*Document, []*memory.MemoryEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if k <= 0 {
		k = r.SearchTopK
	}

	// Search vector store
	docs := r.vectorStore.Search(query, k)

	// Filter by minimum score
	filteredDocs := make([]*Document, 0)
	for _, doc := range docs {
		score := r.computeScore(query, doc)
		if score >= r.MinScore {
			filteredDocs = append(filteredDocs, doc)
		}
	}

	// Get relevant working memory
	working, longTerm := r.memoryMgr.GetContext(query, k, k)

	// Combine results
	_ = longTerm
	_ = working

	return filteredDocs, working, nil
}

// RetrieveWithMetadata performs RAG retrieval with metadata filtering.
func (r *Retriever) RetrieveWithMetadata(query string, k int, filter map[string]string) ([]*Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if k <= 0 {
		k = r.SearchTopK
	}

	docs := r.vectorStore.SearchWithMetadata(query, k, filter)

	filteredDocs := make([]*Document, 0)
	for _, doc := range docs {
		score := r.computeScore(query, doc)
		if score >= r.MinScore {
			filteredDocs = append(filteredDocs, doc)
		}
	}

	return filteredDocs, nil
}

// GetContext builds a context string for LLM prompts.
func (r *Retriever) GetContext(query string, maxContextLen int) (string, error) {
	docs, working, err := r.Retrieve(query, r.SearchTopK)
	if err != nil {
		return "", err
	}

	var ctx strings.Builder
	ctx.WriteString("### Relevant Knowledge\n\n")

	for i, doc := range docs {
		content := doc.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		ctx.WriteString(fmt.Sprintf("%d. %s\n", i+1, content))
	}

	ctx.WriteString("\n### Recent Conversation\n\n")
	for i, entry := range working {
		content := entry.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		ctx.WriteString(fmt.Sprintf("%d. %s\n", i+1, content))
	}

	result := ctx.String()
	if len(result) > maxContextLen {
		result = result[:maxContextLen] + "..."
	}

	return result, nil
}

// SetMinScore sets the minimum similarity score.
func (r *Retriever) SetMinScore(score float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.MinScore = score
}

// SetTopK sets the default number of results.
func (r *Retriever) SetTopK(k int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.SearchTopK = k
}

// Stats returns statistics about the RAG system.
func (r *Retriever) Stats() RAGStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return RAGStats{
		DocumentCount:  r.vectorStore.Len(),
		WorkingCount:   r.memoryMgr.Working.Len(),
		LongTermCount:  r.memoryMgr.LongTerm.Len(),
	}
}

// RAGStats holds statistics about the RAG system.
type RAGStats struct {
	DocumentCount  int
	WorkingCount   int
	LongTermCount  int
}

// Clear clears all data.
func (r *Retriever) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.vectorStore.Clear(); err != nil {
		return err
	}

	return r.memoryMgr.Clear()
}

// Close saves all data.
func (r *Retriever) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.memoryMgr.Close(); err != nil {
		return err
	}

	return nil
}

// computeScore computes a relevance score for a document.
func (r *Retriever) computeScore(query string, doc *Document) float64 {
	// Simple scoring: boost by access count and recency
	baseScore := 1.0

	// Boost by access count
	if accessCount, ok := doc.Metadata["_access_count"]; ok {
		if n, err := strconv.Atoi(accessCount); err == nil {
			baseScore += float64(n) * 0.1
		}
	}

	// Boost by recency (within last 24 hours)
	if updatedAt, ok := doc.Metadata["_updated_at"]; ok {
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			hoursSince := time.Since(t).Hours()
			if hoursSince < 24 {
				baseScore += 0.2
			} else if hoursSince < 7*24 {
				baseScore += 0.1
			}
		}
	}

	return baseScore
}

// ImportDocuments imports documents from external source.
func (r *Retriever) ImportDocuments(contents []string, sources []string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for i, content := range contents {
		metadata := map[string]string{
			"source": "import",
		}
		if i < len(sources) {
			metadata["source_id"] = sources[i]
		}

		_, err := r.vectorStore.AddDocument(content, metadata)
		if err == nil {
			count++
		}
	}

	if r.vectorStore.storePath != "" {
		r.vectorStore.save()
	}

	return count, nil
}

// ExportDocuments exports all documents.
func (r *Retriever) ExportDocuments() []*Document {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.vectorStore.GetAll()
}

// DeleteDocument removes a document.
func (r *Retriever) DeleteDocument(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.vectorStore.DeleteDocument(id)
}

// GetDocument retrieves a document.
func (r *Retriever) GetDocument(id string) (*Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.vectorStore.GetDocument(id)
}
