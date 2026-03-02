package core

import "time"

// ImageAttachment represents an image sent by the user.
type ImageAttachment struct {
	MimeType string // e.g. "image/png", "image/jpeg"
	Data     []byte // raw image bytes
	FileName string // original filename (optional)
}

// AudioAttachment represents a voice/audio message sent by the user.
type AudioAttachment struct {
	MimeType string // e.g. "audio/amr", "audio/ogg", "audio/mp4"
	Data     []byte // raw audio bytes
	Format   string // short format hint: "amr", "ogg", "m4a", "mp3", "wav", etc.
	Duration int    // duration in seconds (if known)
}

// Message represents a unified incoming message from any platform.
type Message struct {
	SessionKey string // unique key for user context, e.g. "feishu:{chatID}:{userID}"
	Platform   string
	UserID     string
	UserName   string
	Content    string
	Images     []ImageAttachment // attached images (if any)
	Audio      *AudioAttachment  // voice message (if any)
	ReplyCtx   any               // platform-specific context needed for replying
}

// EventType distinguishes different kinds of agent output.
type EventType string

const (
	EventText              EventType = "text"               // intermediate or final text
	EventToolUse           EventType = "tool_use"           // tool invocation info
	EventToolResult        EventType = "tool_result"        // tool execution result
	EventResult            EventType = "result"             // final aggregated result
	EventError             EventType = "error"              // error occurred
	EventPermissionRequest EventType = "permission_request" // agent requests permission via stdio protocol
	EventThinking          EventType = "thinking"           // thinking/processing status
	EventAPIStats          EventType = "api_stats"          // API call statistics
)

// Event represents a single piece of agent output streamed back to the engine.
type Event struct {
	Type         EventType
	Content      string
	ToolName     string         // populated for EventToolUse, EventPermissionRequest
	ToolInput    string         // human-readable summary of tool input
	ToolInputRaw map[string]any // raw tool input (for EventPermissionRequest, used in allow response)
	ToolResult   string         // populated for EventToolResult
	SessionID    string         // agent-managed session ID for conversation continuity
	RequestID    string         // unique request ID for EventPermissionRequest
	// API statistics fields (populated for EventAPIStats)
	APICallStats *APIStats   `json:"api_call_stats,omitempty"`
	TokenUsage   *TokenUsage `json:"token_usage,omitempty"`
	Done         bool
	Error        error
}

// HistoryEntry is one turn in a conversation.
type HistoryEntry struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// AgentSessionInfo describes one session as reported by the agent backend.
type AgentSessionInfo struct {
	ID           string
	Summary      string
	MessageCount int
	ModifiedAt   time.Time
	GitBranch    string
}

// APIStats holds API call statistics for a session.
type APIStats struct {
	TotalCalls      int64                         `json:"total_calls"`
	SuccessfulCalls int64                         `json:"successful_calls"`
	FailedCalls     int64                         `json:"failed_calls"`
	TokensUsed      TokenUsage                    `json:"tokens_used"`
	TokensInput     TokensInput                   `json:"tokens_input"`
	TokensOutput    TokensOutput                  `json:"tokens_output"`
	StartTime       time.Time                     `json:"start_time"`
	LastCallTime    *time.Time                    `json:"last_call_time,omitempty"`
	ProviderStats   map[string]*ProviderCallStats `json:"provider_stats"`
	ModelStats      map[string]*ModelStats        `json:"model_stats"` // Statistics grouped by model
}

// TokenUsage represents token consumption statistics.
type TokenUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// TokensInput represents input token usage statistics.
type TokensInput struct {
	TextTokens   int64            `json:"text_tokens"`
	ImageTokens  int64            `json:"image_tokens"`
	CachedTokens int64            `json:"cached_tokens"`
	Details      map[string]int64 `json:"details,omitempty"` // Additional input token details
}

// TokensOutput represents output token usage statistics.
type TokensOutput struct {
	TextTokens  int64            `json:"text_tokens"`
	ImageTokens int64            `json:"image_tokens"`
	Details     map[string]int64 `json:"details,omitempty"` // Additional output token details
}

// ProviderCallStats holds statistics for a specific provider.
type ProviderCallStats struct {
	Calls           int64      `json:"calls"`
	Errors          int64      `json:"errors"`
	TokensUsed      TokenUsage `json:"tokens_used"`
	AvgResponseTime float64    `json:"avg_response_time"` // in seconds
}

// ModelStats holds statistics for a specific model.
type ModelStats struct {
	ModelName    string       `json:"model_name"`
	Calls        int64        `json:"calls"`
	TokensUsed   TokenUsage   `json:"tokens_used"`
	TokensInput  TokensInput  `json:"tokens_input"`
	TokensOutput TokensOutput `json:"tokens_output"`
	StartTime    time.Time    `json:"start_time"`
	LastCallTime *time.Time   `json:"last_call_time,omitempty"`
}
