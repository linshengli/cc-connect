// Package sandbox provides a safe execution environment for running
// untrusted code and commands with resource limits and isolation using Docker containers.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	CPULimit      float64           // CPU limit (0 = unlimited, 1.0 = 1 CPU core)
	NetworkAccess bool              // Allow network access
	EnvWhitelist  []string          // Allowed environment variables
	FileAccess    FileAccessMode    // File access mode
	AllowedCmds   []string          // Allowed commands (empty = all)
	MaxOutputSize int64             // Max output size in bytes
	TempDir       string            // Directory for temp files
	UseDocker     bool              // Use Docker container for isolation
	DockerImage   string            // Docker image to use (default: alpine)
	ContainerID   string            // Container ID if using existing container
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
	CPUUsed    float64
	Error      error
}

// Sandbox provides isolated execution environment using Docker containers.
type Sandbox struct {
	config      *Config
	mu          sync.Mutex
	containerID string
	workDir     string
	tempDir     string
	cleanupFunc func()
}

// DockerConfig represents Docker container configuration.
type DockerConfig struct {
	Image       string
	MemoryLimit string
	CPULimit    string
	Network     string
	WorkDir     string
	Binds       []string
	Env         []string
}

// New creates a new sandbox with optional Docker container isolation.
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
	if cfg.DockerImage == "" {
		cfg.DockerImage = "alpine:latest"
	}
	if cfg.UseDocker && !isDockerAvailable() {
		// Fall back to process isolation if Docker is not available
		cfg.UseDocker = false
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

	s := &Sandbox{
		config:  cfg,
		tempDir: sandboxDir,
		workDir: cfg.WorkDir,
	}

	// Initialize Docker container if requested
	if cfg.UseDocker {
		if err := s.initDockerContainer(); err != nil {
			// Fall back to process isolation
			cfg.UseDocker = false
		}
	}

	return s, nil
}

// isDockerAvailable checks if Docker daemon is running and accessible.
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
	err := cmd.Run()
	return err == nil
}

// initDockerContainer creates and starts a Docker container.
func (s *Sandbox) initDockerContainer() error {
	dockerCfg := s.buildDockerConfig()

	// Build docker run command
	args := []string{
		"run",
		"-d", // Detached mode
		"--rm", // Auto cleanup
		"--workdir", dockerCfg.WorkDir,
	}

	// Add memory limit
	if s.config.MemoryLimit > 0 {
		args = append(args, "--memory", formatMemoryLimit(s.config.MemoryLimit))
	}

	// Add CPU limit
	if s.config.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", s.config.CPULimit))
	}

	// Network configuration
	if !s.config.NetworkAccess {
		args = append(args, "--network", "none")
	} else {
		args = append(args, "--network", "bridge")
	}

	// Mount work directory
	args = append(args, "-v", fmt.Sprintf("%s:%s", s.workDir, dockerCfg.WorkDir))

	// Mount temp directory for temporary files
	args = append(args, "-v", fmt.Sprintf("%s:/tmp", s.tempDir))

	// Security options
	args = append(args,
		"--cap-drop=ALL", // Drop all capabilities
		"--read-only",    // Read-only root filesystem
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=16m",
	)

	// Add image and command
	args = append(args, dockerCfg.Image)
	args = append(args, "sleep", "infinity") // Keep container running

	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	s.containerID = strings.TrimSpace(string(output))
	return nil
}

// buildDockerConfig builds Docker configuration from sandbox config.
func (s *Sandbox) buildDockerConfig() *DockerConfig {
	cfg := &DockerConfig{
		Image:   s.config.DockerImage,
		WorkDir: "/workspace",
		Env:     s.filterEnv(),
		Binds: []string{
			fmt.Sprintf("%s:%s", s.workDir, "/workspace"),
		},
	}

	if s.config.MemoryLimit > 0 {
		cfg.MemoryLimit = formatMemoryLimit(s.config.MemoryLimit)
	}

	if s.config.CPULimit > 0 {
		cfg.CPULimit = fmt.Sprintf("%.2f", s.config.CPULimit)
	}

	if s.config.NetworkAccess {
		cfg.Network = "bridge"
	} else {
		cfg.Network = "none"
	}

	return cfg
}

// formatMemoryLimit converts bytes to Docker memory format.
func formatMemoryLimit(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	if bytes >= GB {
		return fmt.Sprintf("%.1fg", float64(bytes)/GB)
	}
	if bytes >= MB {
		return fmt.Sprintf("%.1fm", float64(bytes)/MB)
	}
	if bytes >= KB {
		return fmt.Sprintf("%.1fk", float64(bytes)/KB)
	}
	return fmt.Sprintf("%d", bytes)
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

	startTime := time.Now()

	var result *Result
	var err error

	if s.config.UseDocker && s.containerID != "" {
		result, err = s.runInDocker(ctx, name, args)
	} else {
		result, err = s.runInProcess(ctx, name, args)
	}

	if result != nil {
		result.Duration = time.Since(startTime)
	}

	return result, err
}

// runInDocker executes a command inside a Docker container.
func (s *Sandbox) runInDocker(ctx context.Context, name string, args []string) (*Result, error) {
	// Build docker exec command
	execArgs := []string{
		"exec",
		"-i", // Interactive
		"-w", "/workspace",
	}

	// Add environment variables
	for _, env := range s.filterEnv() {
		execArgs = append(execArgs, "-e", env)
	}

	execArgs = append(execArgs, s.containerID)
	execArgs = append(execArgs, name)
	execArgs = append(execArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", execArgs...)

	// Create pipes for stdout/stderr
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	// Set process group for cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return &Result{
			ExitCode: -1,
			Error:    fmt.Errorf("failed to start: %w", err),
			TimedOut: ctx.Err() == context.DeadlineExceeded,
		}, nil
	}

	// Wait for completion
	err := cmd.Wait()

	result := &Result{
		Stdout: stdoutBuf.Bytes(),
		Stderr: stderrBuf.Bytes(),
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

	// Get resource usage from Docker
	s.updateResourceUsage(result)

	return result, nil
}

// runInProcess executes a command using process-level isolation (fallback).
func (s *Sandbox) runInProcess(ctx context.Context, name string, args []string) (*Result, error) {
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

	result := &Result{
		Stdout: stdoutData,
		Stderr: stderrData,
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

// updateResourceUsage updates resource usage from Docker stats.
func (s *Sandbox) updateResourceUsage(result *Result) {
	if !s.config.UseDocker || s.containerID == "" {
		return
	}

	// Get Docker stats
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{.MemUsage}}|{{.CPUPerc}}", s.containerID)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) >= 2 {
		result.MemoryUsed = parseMemoryUsage(parts[0])
		result.CPUUsed = parseCPUPercent(parts[1])
	}
}

// parseMemoryUsage parses Docker memory usage string.
func parseMemoryUsage(memStr string) int64 {
	memStr = strings.TrimSpace(memStr)
	// Format: "100MiB / 1GiB" or "100MB / 1GB"
	parts := strings.Split(memStr, "/")
	if len(parts) == 0 {
		return 0
	}

	usage := strings.TrimSpace(parts[0])
	var multiplier int64 = 1

	if strings.Contains(usage, "GiB") || strings.Contains(usage, "GB") {
		multiplier = 1024 * 1024 * 1024
		usage = strings.ReplaceAll(usage, "GiB", "")
		usage = strings.ReplaceAll(usage, "GB", "")
	} else if strings.Contains(usage, "MiB") || strings.Contains(usage, "MB") {
		multiplier = 1024 * 1024
		usage = strings.ReplaceAll(usage, "MiB", "")
		usage = strings.ReplaceAll(usage, "MB", "")
	} else if strings.Contains(usage, "KiB") || strings.Contains(usage, "KB") {
		multiplier = 1024
		usage = strings.ReplaceAll(usage, "KiB", "")
		usage = strings.ReplaceAll(usage, "KB", "")
	}

	var val float64
	fmt.Sscanf(strings.TrimSpace(usage), "%f", &val)
	return int64(val * float64(multiplier))
}

// parseCPUPercent parses Docker CPU percentage string.
func parseCPUPercent(cpuStr string) float64 {
	cpuStr = strings.TrimSpace(cpuStr)
	cpuStr = strings.TrimSuffix(cpuStr, "%")

	var val float64
	fmt.Sscanf(cpuStr, "%f", &val)
	return val
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

	// Stop Docker container if using Docker
	if s.config.UseDocker && s.containerID != "" {
		cmd := exec.Command("docker", "stop", s.containerID)
		cmd.Run()
		s.containerID = ""
	}

	// Clean up temp directory
	if s.tempDir != "" {
		return os.RemoveAll(s.tempDir)
	}
	return nil
}

// GetWorkDir returns the working directory.
func (s *Sandbox) GetWorkDir() string {
	return s.workDir
}

// GetTempDir returns the temp directory.
func (s *Sandbox) GetTempDir() string {
	return s.tempDir
}

// GetContainerID returns the Docker container ID if using Docker.
func (s *Sandbox) GetContainerID() string {
	return s.containerID
}

// SetWorkDir sets the working directory.
func (s *Sandbox) SetWorkDir(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	s.workDir = dir
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
		fullPath = filepath.Join(s.workDir, path)
	}

	if !s.isPathAllowed(fullPath) {
		return fmt.Errorf("path not allowed: %s", path)
	}

	// If using Docker, write to the mounted directory
	return os.WriteFile(fullPath, data, perm)
}

// ReadFile reads a file from the sandbox.
func (s *Sandbox) ReadFile(path string) ([]byte, error) {
	if s.config.FileAccess == FileAccessNone {
		return nil, fmt.Errorf("file access not allowed")
	}

	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(s.workDir, path)
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
		fullPath = filepath.Join(s.workDir, path)
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

// CopyToContainer copies a file from host to Docker container.
func (s *Sandbox) CopyToContainer(hostPath, containerPath string) error {
	if !s.config.UseDocker || s.containerID == "" {
		return fmt.Errorf("not using Docker")
	}

	cmd := exec.Command("docker", "cp", hostPath, fmt.Sprintf("%s:%s", s.containerID, containerPath))
	return cmd.Run()
}

// CopyFromContainer copies a file from Docker container to host.
func (s *Sandbox) CopyFromContainer(containerPath, hostPath string) error {
	if !s.config.UseDocker || s.containerID == "" {
		return fmt.Errorf("not using Docker")
	}

	cmd := exec.Command("docker", "cp", fmt.Sprintf("%s:%s", s.containerID, containerPath), hostPath)
	return cmd.Run()
}

// GetDockerLogs gets logs from the Docker container.
func (s *Sandbox) GetDockerLogs(tail int) ([]byte, error) {
	if !s.config.UseDocker || s.containerID == "" {
		return nil, fmt.Errorf("not using Docker")
	}

	args := []string{"logs", s.containerID}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := exec.Command("docker", args...)
	return cmd.Output()
}

// ExecInContainer executes a command in the running container and returns output.
func (s *Sandbox) ExecInContainer(cmdArgs ...string) ([]byte, error) {
	if !s.config.UseDocker || s.containerID == "" {
		return nil, fmt.Errorf("not using Docker")
	}

	args := append([]string{"exec", s.containerID}, cmdArgs...)
	cmd := exec.Command("docker", args...)
	return cmd.Output()
}

// Cleanup removes all temporary files and stops Docker container.
func (s *Sandbox) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop Docker container if using Docker
	if s.config.UseDocker && s.containerID != "" {
		cmd := exec.Command("docker", "stop", s.containerID)
		if err := cmd.Run(); err != nil {
			// Force remove if stop fails
			cmd = exec.Command("docker", "rm", "-f", s.containerID)
			cmd.Run()
		}
		s.containerID = ""
	}

	// Clean up temp directory
	if s.tempDir != "" {
		return os.RemoveAll(s.tempDir)
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
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=/tmp",
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
		return strings.HasPrefix(absPath, s.workDir) ||
			strings.HasPrefix(absPath, s.tempDir)
	case FileAccessWorkDirOnly:
		return strings.HasPrefix(absPath, s.workDir)
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
		Docker:   isDockerAvailable(),
	}
}

// RuntimeInfo holds runtime environment information.
type RuntimeInfo struct {
	OS       string
	Arch     string
	NumCPU   int
	NumGorun int
	Docker   bool
}

// DefaultConfig returns a safe default configuration with Docker enabled.
func DefaultConfig() *Config {
	return &Config{
		Timeout:       30 * time.Second,
		MemoryLimit:   512 * 1024 * 1024, // 512MB
		CPULimit:      1.0,               // 1 CPU core
		NetworkAccess: false,
		FileAccess:    FileAccessWorkDirOnly,
		MaxOutputSize: 10 * 1024 * 1024, // 10MB
		AllowedCmds:   []string{"ls", "cat", "grep", "find", "go", "python3", "node", "npm"},
		UseDocker:     true,
		DockerImage:   "alpine:latest",
	}
}

// SecureConfig returns a highly restrictive configuration with Docker.
func SecureConfig() *Config {
	return &Config{
		Timeout:       10 * time.Second,
		MemoryLimit:   128 * 1024 * 1024, // 128MB
		CPULimit:      0.5,               // 0.5 CPU core
		NetworkAccess: false,
		FileAccess:    FileAccessWorkDirOnly,
		MaxOutputSize: 1 * 1024 * 1024, // 1MB
		AllowedCmds:   []string{"ls", "cat", "grep"},
		EnvWhitelist:  []string{"PATH", "HOME"},
		UseDocker:     true,
		DockerImage:   "alpine:latest",
	}
}

// ContainerInfo returns information about the sandbox container.
type ContainerInfo struct {
	ID           string `json:"id"`
	Image        string `json:"image"`
	MemoryLimit  int64  `json:"memory_limit"`
	CPULimit     float64 `json:"cpu_limit"`
	NetworkAccess bool  `json:"network_access"`
	WorkDir      string `json:"work_dir"`
}

// GetContainerInfo returns information about the current container.
func (s *Sandbox) GetContainerInfo() *ContainerInfo {
	return &ContainerInfo{
		ID:            s.containerID,
		Image:         s.config.DockerImage,
		MemoryLimit:   s.config.MemoryLimit,
		CPULimit:      s.config.CPULimit,
		NetworkAccess: s.config.NetworkAccess,
		WorkDir:       s.workDir,
	}
}

// InspectContainer returns detailed container information from Docker.
func (s *Sandbox) InspectContainer() (map[string]interface{}, error) {
	if !s.config.UseDocker || s.containerID == "" {
		return nil, fmt.Errorf("not using Docker")
	}

	cmd := exec.Command("docker", "inspect", s.containerID)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	if len(result) > 0 {
		return result[0], nil
	}
	return nil, fmt.Errorf("no container info found")
}
