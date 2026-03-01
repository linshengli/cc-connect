# New Features Documentation

This document describes the new features added to cc-connect.

## Table of Contents

1. [Memory Management (RAG)](#memory-management---rag)
2. [Load Balancer](#load-balancer)
3. [Channel Management](#channel-management)
4. [Sandbox](#sandbox)
5. [Configuration Hot-Reload](#configuration-hot-reload)

---

## Memory Management - RAG

### Overview

The memory module provides short-term and long-term memory management with automatic compression for AI conversations.

### Components

#### WorkingMemory (Short-term)
- In-memory storage for recent conversation turns
- Configurable entry limit and token limit
- Automatic eviction of oldest entries when limits exceeded
- Access count tracking for relevance scoring

#### LongTermMemory (Long-term)
- Persistent disk-backed storage (JSON)
- Search functionality for retrieving memories
- Metadata-based categorization
- Automatic pruning when max entries exceeded

#### Compressor
- Compresses working memory entries to long-term storage
- Default concatenation with categorization
- Custom compression function support
- Triggered when entry threshold reached

#### MemoryManager
- Coordinates working memory, long-term memory, and compression
- Automatic compression on message add
- Context retrieval for queries

### Usage

```go
import "github.com/chenhg5/cc-connect/memory"

// Create memory manager
mm, err := memory.NewMemoryManager("/path/to/session")
if err != nil {
    log.Fatal(err)
}
defer mm.Close()

// Add message (auto-triggers compression if needed)
entry, evicted, err := mm.AddMessage("User query", map[string]string{
    "category": "question",
})

// Get context for LLM
working, longTerm := mm.GetContext("Go programming", 10, 5)

// Get recent conversation
recent := mm.GetRecentContext(20)
```

---

## RAG (Retrieval-Augmented Generation)

### Overview

The RAG module provides semantic search capabilities using TF-IDF vectorization and cosine similarity.

### Components

#### VectorStore
- TF-IDF based vectorization
- Cosine similarity search
- Metadata filtering
- Persistent storage

#### Retriever
- Combines vector search with memory
- Configurable top-K results
- Minimum score threshold
- Context building for LLM prompts

### Usage

```go
import "github.com/chenhg5/cc-connect/rag"

// Create retriever
r, err := rag.NewRetriever("/path/to/session")
if err != nil {
    log.Fatal(err)
}
defer r.Close()

// Add knowledge
doc, err := r.AddKnowledge("Go is a statically typed language", map[string]string{
    "category": "programming",
})

// Retrieve relevant context
docs, working, err := r.Retrieve("Go language", 5)

// Build context for prompt
ctx, err := r.GetContext("Go programming", 2000)
```

---

## Load Balancer

### Overview

The load balancer module provides multi-provider load balancing with automatic failover.

### Strategies

- **Round Robin**: Even distribution
- **Weighted Round Robin**: Traffic by weight
- **Priority**: Highest priority healthy provider
- **Least Errors**: Fewest consecutive errors
- **Random**: Random healthy provider

### Features

- Health tracking (healthy/degraded/unhealthy)
- Automatic failover on consecutive errors
- Periodic health checks
- Provider statistics

### Usage

```go
import "github.com/chenhg5/cc-connect/loadbalancer"

// Create balancer
b := loadbalancer.NewBalancer(loadbalancer.StrategyRoundRobin)

// Add providers
b.AddProvider(&loadbalancer.Provider{
    Name:     "openai",
    BaseURL:  "https://api.openai.com",
    APIKey:   "sk-...",
    Weight:   2,
    Priority: 1,
})

// Get provider for request
provider, err := b.GetProvider()
if err != nil {
    log.Fatal(err)
}

// Report result
if success {
    b.ReportSuccess(provider.Name)
} else {
    b.ReportError(provider.Name, err)
}
```

---

## Channel Management

### Overview

The channel module manages multiple concurrent communication channels with rate limiting and state control.

### Features

- Per-channel rate limiting
- Global concurrent request limiting
- Channel states: active/paused/closed
- Session-to-channel mapping

### Usage

```go
import "github.com/chenhg5/cc-connect/channel"

// Create manager
m := channel.NewManager(10) // max 10 global concurrent

// Get or create channel
ch := m.GetOrCreateChannel("feishu", "user123", "User Name")

// Acquire slot for processing
ctx, cancel, err := m.Acquire("feishu", "user123")
if err != nil {
    // Rate limited or not active
}
defer func() {
    cancel()
    m.Release()
}()

// Pause/Resume channel
m.PauseChannel("feishu", "user123")
m.ResumeChannel("feishu", "user123")
```

---

## Sandbox

### Overview

The sandbox module provides a secure execution environment for running untrusted code.

### Security Features

- Command whitelist
- File access control (None/ReadOnly/WorkDirOnly/Full)
- Environment filtering
- Timeout enforcement
- Output size limits
- Process isolation

### Usage

```go
import "github.com/chenhg5/cc-connect/sandbox"

// Create sandbox with default config
cfg := sandbox.DefaultConfig()
s, err := sandbox.New(cfg)
if err != nil {
    log.Fatal(err)
}
defer s.Cleanup()

// Run command
result, err := s.Run(ctx, "echo", "hello")
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(result.Stdout))

// Run code
result, err = s.RunCode(ctx, "print('Hello')", "python3")

// File operations
s.WriteFile("test.txt", []byte("content"), 0644)
content, _ := s.ReadFile("test.txt")
```

### Configurations

```go
// Default: balanced security
cfg := sandbox.DefaultConfig()

// Secure: highly restrictive
cfg := sandbox.SecureConfig()

// Custom
cfg := &sandbox.Config{
    Timeout:       30 * time.Second,
    MemoryLimit:   512 * 1024 * 1024,
    NetworkAccess: false,
    FileAccess:    sandbox.FileAccessWorkDirOnly,
    AllowedCmds:   []string{"ls", "cat", "grep", "go", "python3"},
}
```

---

## Configuration Hot-Reload

### Overview

The hot-reload feature automatically detects and reloads configuration changes without restarting the service.

### Features

- Periodic file monitoring
- Automatic reload on change
- Callback support for change notifications
- Thread-safe access

### Usage

```go
import "github.com/chenhg5/cc-connect/config"

// Create hot reloader
hr := config.NewHotReloader("/path/to/config.toml", 100*time.Millisecond)

// Set change callback
hr.SetOnChange(func(cfg *config.Config) {
    log.Println("Configuration changed!")
    // Update running services with new config
})

// Start watching
if err := hr.Start(); err != nil {
    log.Fatal(err)
}
defer hr.Stop()

// Get current config
cfg := hr.GetConfig()

// Force reload
hr.Reload()
```

### Dynamic Configuration

```go
// Update providers for a project
cfg.UpdateProjectProviders("my-project", providers)

// Update agent options
cfg.SetProjectAgentOption("my-project", "mode", "yolo")
```

---

## Testing

All new features include comprehensive test coverage:

- **Memory**: 25+ test cases
- **RAG**: 36+ test cases
- **Load Balancer**: 23+ test cases
- **Channel**: 27+ test cases
- **Sandbox**: 25+ test cases
- **Config**: 7+ test cases
- **Core (i18n, registry)**: 25+ test cases

Run all tests:
```bash
go test ./...
```

Run specific package tests:
```bash
go test ./memory/... -v
go test ./rag/... -v
go test ./loadbalancer/... -v
go test ./channel/... -v
go test ./sandbox/... -v
go test ./config/... -v
go test ./core/... -v
```
