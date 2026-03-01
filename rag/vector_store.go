// Package rag implements Retrieval-Augmented Generation (RAG) capabilities
// for semantic search and knowledge retrieval in AI conversations.
package rag

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Vector represents a simple TF-IDF weighted vector for a document.
type Vector struct {
	Weights map[string]float64 // term -> weight
	Norm    float64            // L2 norm for cosine similarity
}

// Document represents a retrievable document chunk.
type Document struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata"`
	Vector    *Vector           `json:"-"` // not serialized, computed on load
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// VectorStore holds documents and supports semantic search.
type VectorStore struct {
	mu          sync.RWMutex
	documents   map[string]*Document
	idf         map[string]float64 // inverse document frequency
	vocab       map[string]bool    // vocabulary
	storePath   string
	TopK        int // default number of results to return
}

// NewVectorStore creates a new vector store.
func NewVectorStore(storePath string) (*VectorStore, error) {
	vs := &VectorStore{
		documents: make(map[string]*Document),
		idf:       make(map[string]float64),
		vocab:     make(map[string]bool),
		storePath: storePath,
		TopK:      5,
	}

	if err := vs.load(); err != nil {
		return nil, err
	}

	return vs, nil
}

// AddDocument adds a document to the vector store.
func (vs *VectorStore) AddDocument(content string, metadata map[string]string) (*Document, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	doc := &Document{
		ID:        fmt.Sprintf("doc_%d_%s", time.Now().UnixNano(), randomID(6)),
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Compute vector
	doc.Vector = vs.computeVector(content)

	vs.documents[doc.ID] = doc
	vs.updateIDF()

	if vs.storePath != "" {
		if err := vs.save(); err != nil {
			return nil, err
		}
	}

	return doc, nil
}

// AddDocuments adds multiple documents in batch.
func (vs *VectorStore) AddDocuments(contents []string, metadata []map[string]string) ([]*Document, error) {
	if len(metadata) == 0 {
		metadata = make([]map[string]string, len(contents))
	}

	vs.mu.Lock()
	defer vs.mu.Unlock()

	docs := make([]*Document, 0, len(contents))

	for i, content := range contents {
		doc := &Document{
			ID:        fmt.Sprintf("doc_%d_%s", time.Now().UnixNano(), randomID(6)),
			Content:   content,
			Metadata:  metadata[i],
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		doc.Vector = vs.computeVector(content)
		vs.documents[doc.ID] = doc
		docs = append(docs, doc)
	}

	vs.updateIDF()

	if vs.storePath != "" {
		if err := vs.save(); err != nil {
			return nil, err
		}
	}

	return docs, nil
}

// Search performs semantic search and returns top-K most relevant documents.
func (vs *VectorStore) Search(query string, k int) []*Document {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if k <= 0 {
		k = vs.TopK
	}

	if len(vs.documents) == 0 {
		return nil
	}

	// Compute query vector
	queryVec := vs.computeVector(query)

	// Compute similarities
	type scoredDoc struct {
		doc   *Document
		score float64
	}

	scored := make([]scoredDoc, 0, len(vs.documents))
	for _, doc := range vs.documents {
		score := cosineSimilarity(queryVec, doc.Vector)
		if score > 0 {
			scored = append(scored, scoredDoc{doc: doc, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Return top-K
	result := make([]*Document, 0, k)
	for i := 0; i < len(scored) && i < k; i++ {
		result = append(result, scored[i].doc)
	}

	return result
}

// SearchWithMetadata performs search with metadata filtering.
func (vs *VectorStore) SearchWithMetadata(query string, k int, filter map[string]string) []*Document {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if k <= 0 {
		k = vs.TopK
	}

	queryVec := vs.computeVector(query)

	type scoredDoc struct {
		doc   *Document
		score float64
	}

	scored := make([]scoredDoc, 0)

	for _, doc := range vs.documents {
		// Apply metadata filter
		if !matchesFilter(doc.Metadata, filter) {
			continue
		}

		score := cosineSimilarity(queryVec, doc.Vector)
		if score > 0 {
			scored = append(scored, scoredDoc{doc: doc, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]*Document, 0, k)
	for i := 0; i < len(scored) && i < k; i++ {
		result = append(result, scored[i].doc)
	}

	return result
}

// GetDocument retrieves a document by ID.
func (vs *VectorStore) GetDocument(id string) (*Document, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	doc, ok := vs.documents[id]
	if !ok {
		return nil, fmt.Errorf("document %s not found", id)
	}

	return doc, nil
}

// DeleteDocument removes a document by ID.
func (vs *VectorStore) DeleteDocument(id string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	delete(vs.documents, id)

	if vs.storePath != "" {
		return vs.save()
	}

	return nil
}

// Len returns the number of documents.
func (vs *VectorStore) Len() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return len(vs.documents)
}

// GetAll returns all documents.
func (vs *VectorStore) GetAll() []*Document {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	result := make([]*Document, 0, len(vs.documents))
	for _, doc := range vs.documents {
		result = append(result, doc)
	}
	return result
}

// Clear removes all documents.
func (vs *VectorStore) Clear() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.documents = make(map[string]*Document)
	vs.idf = make(map[string]float64)
	vs.vocab = make(map[string]bool)

	if vs.storePath != "" {
		return vs.save()
	}

	return nil
}

// tokenize splits text into tokens with basic normalization.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Remove punctuation and split
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if isAlphaNumeric(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func isAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 0x4E00 && r <= 0x9FFF)
}

// computeVector computes a TF-IDF vector for text.
func (vs *VectorStore) computeVector(text string) *Vector {
	tokens := tokenize(text)

	// Compute term frequency
	tf := make(map[string]float64)
	for _, token := range tokens {
		tf[token]++
	}

	// Normalize TF and apply IDF
	weights := make(map[string]float64)
	for term, count := range tf {
		tfNorm := count / float64(len(tokens))
		idf := vs.idf[term]
		if idf == 0 {
			idf = 1.0 // default for new terms
		}
		weights[term] = tfNorm * idf
		vs.vocab[term] = true
	}

	// Compute L2 norm
	norm := 0.0
	for _, w := range weights {
		norm += w * w
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		norm = 1e-10 // avoid division by zero
	}

	return &Vector{
		Weights: weights,
		Norm:    norm,
	}
}

// updateIDF recomputes inverse document frequency for all terms.
func (vs *VectorStore) updateIDF() {
	// Count document frequency for each term
	df := make(map[string]int)

	for _, doc := range vs.documents {
		seen := make(map[string]bool)
		tokens := tokenize(doc.Content)
		for _, token := range tokens {
			if !seen[token] {
				df[token]++
				seen[token] = true
			}
		}
	}

	// Compute IDF: log(N / df)
	N := float64(len(vs.documents))
	for term, count := range df {
		vs.idf[term] = math.Log(N / float64(count))
	}
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b *Vector) float64 {
	if a == nil || b == nil {
		return 0
	}

	dotProduct := 0.0
	for term, weightA := range a.Weights {
		if weightB, ok := b.Weights[term]; ok {
			dotProduct += weightA * weightB
		}
	}

	// Vectors are already normalized during construction
	return dotProduct / (a.Norm * b.Norm)
}

// matchesFilter checks if document metadata matches all filter criteria.
func matchesFilter(metadata, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}

	for key, value := range filter {
		if metadata[key] != value {
			return false
		}
	}

	return true
}

func (vs *VectorStore) save() error {
	if vs.storePath == "" {
		return nil
	}

	// Create serializable snapshot (without vectors)
	type docSnapshot struct {
		ID        string            `json:"id"`
		Content   string            `json:"content"`
		Metadata  map[string]string `json:"metadata"`
		CreatedAt time.Time         `json:"created_at"`
		UpdatedAt time.Time         `json:"updated_at"`
	}

	snapshot := make([]docSnapshot, 0, len(vs.documents))
	for _, doc := range vs.documents {
		snapshot = append(snapshot, docSnapshot{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  doc.Metadata,
			CreatedAt: doc.CreatedAt,
			UpdatedAt: doc.UpdatedAt,
		})
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal documents: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(vs.storePath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(vs.storePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (vs *VectorStore) load() error {
	if vs.storePath == "" {
		return nil
	}

	data, err := os.ReadFile(vs.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read file: %w", err)
	}

	var snapshot []docSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal documents: %w", err)
	}

	for _, s := range snapshot {
		doc := &Document{
			ID:        s.ID,
			Content:   s.Content,
			Metadata:  s.Metadata,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
		}
		doc.Vector = vs.computeVector(doc.Content)
		vs.documents[doc.ID] = doc
	}

	vs.updateIDF()
	return nil
}

// docSnapshot is used for serialization.
type docSnapshot struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func randomID(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
