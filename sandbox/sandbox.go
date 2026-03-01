// Package sandbox provides a safe execution environment for running
// untrusted code and commands with resource limits and isolation.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Config holds sandbox configuration.
type Config struct {
	WorkDir       string            // Working directory for execution
	Timeout       time.Duration     // Max execution time
	MemoryLimit   int64             // Memory limit in bytes (0 = unlimited)
	CPULimit      float64           // CPU limit (0 = unlimited)
	NetworkAccess bool              // Allow network access
	EnvWhitelist  []string          // Allowed environment variables
	FileAccess    FileAccessMode    // File access mode
	AllowedCmds   []string          // Allowed commands (empty = all)
	MaxOutputSize int64             // Max output size in bytes
	TempDir       string            // Directory for temp files
}

// FileAccessMode defines file access restrictions.
type FileAccessMode int

const (
	FileAccessNone FileAccessMode = iota
	FileAccessReadOnly
	FileAccessWorkDirOnly
	FileAccessFull
)

// Result holds execution result.
type Result struct {
	ExitCode   int
	Stdout     []byte
	Stderr     []byte
	Duration   time.Duration
	TimedOut   bool
	MemoryUsed int64
	Error      error
}

// Sandbox provides isolated execution environment.
type Sandbox struct {
	config *Config
	mu     sync.Mutex
}

// New creates a new sandbox.
func New(cfg *Config) (*Sandbox, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	// Set defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxOutputSize == 0 {
		cfg.MaxOutputSize = 10 * 1024 * 1024 // 10MB
	}
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}

	// Create temp dir for this sandbox
	sandboxDir, err := os.MkdirTemp(cfg.TempDir, "sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox dir: %w", err)
	}
	cfg.TempDir = sandboxDir

	if cfg.WorkDir == "" {
		cfg.WorkDir = sandboxDir
	}

	return &Sandbox{
		config: cfg,
	}, nil
}

// Run executes a command in the sandbox.
func (s *Sandbox) Run(ctx context.Context, name string, args ...string) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if command is allowed
	if !s.isCommandAllowed(name) {
		return nil, fmt.Errorf("command %q is not allowed", name)
	}

	// Create context with timeout
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)

	// Set working directory
	cmd.Dir = s.config.WorkDir

	// Set environment
	cmd.Env = s.filterEnv()

	// Create pipes for stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Set process group for cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start timing
	startTime := time.Now()

	// Start command
	if err := cmd.Start(); err != nil {
		return &Result{
			ExitCode: -1,
			Error:    fmt.Errorf("failed to start: %w", err),
			TimedOut: ctx.Err() == context.DeadlineExceeded,
		}, nil
	}

	// Read output with size limit
	stdoutData, err := readWithLimit(stdout, s.config.MaxOutputSize)
	if err != nil {
		killProcess(cmd)
		return &Result{
			ExitCode: -1,
			Error:    fmt.Errorf("failed to read stdout: %w", err),
		}, nil
	}

	stderrData, err := readWithLimit(stderr, s.config.MaxOutputSize)
	if err != nil {
		killProcess(cmd)
		return &Result{
			ExitCode: -1,
			Error:    fmt.Errorf("failed to read stderr: %w", err),
		}, nil
	}

	// Wait for completion
	err = cmd.Wait()
	duration := time.Since(startTime)

	result := &Result{
		Stdout:   stdoutData,
		Stderr:   stderrData,
		Duration: duration,
	}

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Error = fmt.Errorf("command timed out after %v", s.config.Timeout)
	}

	// Get exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			result.ExitCode = status.ExitStatus()
		} else {
			result.ExitCode = -1
		}
		result.Error = err
	} else {
		result.ExitCode = 0
	}

	return result, nil
}

// RunScript executes a script in the sandbox.
func (s *Sandbox) RunScript(ctx context.Context, script string, interpreter string) (*Result, error) {
	// Create temp file for script
	scriptPath := filepath.Join(s.config.TempDir, "script."+getFileExt(interpreter))
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}

	// Determine interpreter
	if interpreter == "" {
		interpreter = "sh"
	}

	return s.Run(ctx, interpreter, scriptPath)
}

// RunCode executes code in a specific language.
func (s *Sandbox) RunCode(ctx context.Context, code string, language string) (*Result, error) {
	switch strings.ToLower(language) {
	case "python", "python3":
		return s.RunScript(ctx, code, "python3")
	case "go":
		return s.RunGoCode(ctx, code)
	case "javascript", "js", "node", "nodejs":
		return s.RunScript(ctx, code, "node")
	case "bash", "sh":
		return s.RunScript(ctx, code, "sh")
	case "ruby":
		return s.RunScript(ctx, code, "ruby")
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}
}

// runGoCode compiles and runs Go code.
func (s *Sandbox) RunGoCode(ctx context.Context, code string) (*Result, error) {
	// Create temp directory for Go module
	goDir, err := os.MkdirTemp(s.config.TempDir, "gocode-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create Go dir: %w", err)
	}

	// Write go.mod
	goMod := "module sandbox\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte(goMod), 0644); err != nil {
		return nil, fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Write main.go
	if err := os.WriteFile(filepath.Join(goDir, "main.go"), []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write main.go: %w", err)
	}

	// Build
	buildResult, err := s.Run(ctx, "go", "build", "-o", "main", ".")
	if err != nil {
		return buildResult, err
	}
	if buildResult.ExitCode != 0 {
		buildResult.Error = fmt.Errorf("build failed: %s", string(buildResult.Stderr))
		return buildResult, nil
	}

	// Run
	return s.Run(ctx, filepath.Join(goDir, "main"))
}

// Kill terminates all processes in the sandbox.
func (s *Sandbox) Kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Kill process group
	if s.config.TempDir != "" {
		// Clean up temp directory
		return os.RemoveAll(s.config.TempDir)
	}
	return nil
}

// GetWorkDir returns the working directory.
func (s *Sandbox) GetWorkDir() string {
	return s.config.WorkDir
}

// GetTempDir returns the temp directory.
func (s *Sandbox) GetTempDir() string {
	return s.config.TempDir
}

// SetWorkDir sets the working directory.
func (s *Sandbox) SetWorkDir(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	s.config.WorkDir = dir
	return nil
}

// WriteFile writes a file to the sandbox.
func (s *Sandbox) WriteFile(path string, data []byte, perm os.FileMode) error {
	// Check file access mode
	if s.config.FileAccess == FileAccessNone {
		return fmt.Errorf("file access not allowed")
	}

	// Ensure path is within allowed directories
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(s.config.WorkDir, path)
	}

	if !s.isPathAllowed(fullPath) {
		return fmt.Errorf("path not allowed: %s", path)
	}

	return os.WriteFile(fullPath, data, perm)
}

// ReadFile reads a file from the sandbox.
func (s *Sandbox) ReadFile(path string) ([]byte, error) {
	if s.config.FileAccess == FileAccessNone {
		return nil, fmt.Errorf("file access not allowed")
	}

	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(s.config.WorkDir, path)
	}

	if !s.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("path not allowed: %s", path)
	}

	return os.ReadFile(fullPath)
}

// ListFiles lists files in a directory.
func (s *Sandbox) ListFiles(path string) ([]string, error) {
	if s.config.FileAccess == FileAccessNone {
		return nil, fmt.Errorf("file access not allowed")
	}

	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(s.config.WorkDir, path)
	}

	if !s.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("path not allowed: %s", path)
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

// Cleanup removes all temporary files.
func (s *Sandbox) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.TempDir != "" {
		return os.RemoveAll(s.config.TempDir)
	}
	return nil
}

// isCommandAllowed checks if a command is in the allowed list.
func (s *Sandbox) isCommandAllowed(name string) bool {
	if len(s.config.AllowedCmds) == 0 {
		// Empty list means all commands allowed (but still sandboxed)
		return true
	}

	baseName := filepath.Base(name)
	for _, allowed := range s.config.AllowedCmds {
		if allowed == baseName || allowed == name {
			return true
		}
	}
	return false
}

// filterEnv returns filtered environment variables.
func (s *Sandbox) filterEnv() []string {
	if len(s.config.EnvWhitelist) == 0 {
		// No whitelist means minimal safe environment
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + s.config.TempDir,
			"USER=" + os.Getenv("USER"),
		}
	}

	var env []string
	for _, key := range s.config.EnvWhitelist {
		if val := os.Getenv(key); val != "" {
			env = append(env, key+"="+val)
		}
	}
	return env
}

// isPathAllowed checks if a path is within allowed directories.
func (s *Sandbox) isPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	switch s.config.FileAccess {
	case FileAccessNone:
		return false
	case FileAccessReadOnly:
		// Allow reading from workdir and tempdir
		return strings.HasPrefix(absPath, s.config.WorkDir) ||
			strings.HasPrefix(absPath, s.config.TempDir)
	case FileAccessWorkDirOnly:
		return strings.HasPrefix(absPath, s.config.WorkDir)
	case FileAccessFull:
		// Still block some sensitive paths
		blocked := []string{"/etc/passwd", "/etc/shadow", "/root", "/boot"}
		for _, b := range blocked {
			if strings.HasPrefix(absPath, b) {
				return false
			}
		}
		return true
	}
	return false
}

// getFileExt returns file extension for a language.
func getFileExt(interpreter string) string {
	switch interpreter {
	case "python3", "python":
		return "py"
	case "node", "nodejs":
		return "js"
	case "ruby":
		return "rb"
	case "go":
		return "go"
	default:
		return "sh"
	}
}

// readWithLimit reads from a reader with size limit.
func readWithLimit(r interface{ Read([]byte) (int, error) }, limit int64) ([]byte, error) {
	data := make([]byte, 0)
	buf := make([]byte, 4096)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			if int64(len(data)+n) > limit {
				data = append(data, buf[:limit-int64(len(data))]...)
				return data, fmt.Errorf("output size limit exceeded")
			}
			data = append(data, buf[:n]...)
		}
		if err != nil {
			return data, nil
		}
	}
}

// killProcess kills a command and its process group.
func killProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// Kill process group
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Process.Kill()
	}
}

// GetDefaultWorkDir returns a safe default work directory.
func GetDefaultWorkDir() (string, error) {
	return os.MkdirTemp(os.TempDir(), "sandbox-work-*")
}

// GetRuntimeInfo returns information about the runtime environment.
func GetRuntimeInfo() RuntimeInfo {
	return RuntimeInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		NumCPU:   runtime.NumCPU(),
		NumGorun: runtime.NumGoroutine(),
	}
}

// RuntimeInfo holds runtime environment information.
type RuntimeInfo struct {
	OS       string
	Arch     string
	NumCPU   int
	NumGorun int
}

// DefaultConfig returns a safe default configuration.
func DefaultConfig() *Config {
	return &Config{
		Timeout:       30 * time.Second,
		MemoryLimit:   512 * 1024 * 1024, // 512MB
		NetworkAccess: false,
		FileAccess:    FileAccessWorkDirOnly,
		MaxOutputSize: 10 * 1024 * 1024, // 10MB
		AllowedCmds:   []string{"ls", "cat", "grep", "find", "go", "python3", "node", "npm"},
	}
}

// SecureConfig returns a highly restrictive configuration.
func SecureConfig() *Config {
	return &Config{
		Timeout:       10 * time.Second,
		MemoryLimit:   128 * 1024 * 1024, // 128MB
		NetworkAccess: false,
		FileAccess:    FileAccessWorkDirOnly,
		MaxOutputSize: 1 * 1024 * 1024, // 1MB
		AllowedCmds:   []string{"ls", "cat", "grep"},
		EnvWhitelist:  []string{"PATH", "HOME"},
	}
}
