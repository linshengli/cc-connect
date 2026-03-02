package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSandbox_New(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		WorkDir: tmpDir,
		Timeout: 5 * time.Second,
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	if s.config.WorkDir == "" {
		t.Error("expected work dir")
	}

	if s.config.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", s.config.Timeout)
	}
}

func TestSandbox_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}

	if cfg.MemoryLimit != 512*1024*1024 {
		t.Errorf("expected memory limit 512MB, got %d", cfg.MemoryLimit)
	}

	if cfg.NetworkAccess {
		t.Error("expected network access disabled")
	}

	if cfg.FileAccess != FileAccessWorkDirOnly {
		t.Error("expected FileAccessWorkDirOnly")
	}
}

func TestSandbox_SecureConfig(t *testing.T) {
	cfg := SecureConfig()

	if cfg.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", cfg.Timeout)
	}

	if cfg.MemoryLimit != 128*1024*1024 {
		t.Errorf("expected memory limit 128MB, got %d", cfg.MemoryLimit)
	}

	if len(cfg.AllowedCmds) > 3 {
		t.Error("expected very limited commands")
	}
}

func TestSandbox_Run_Echo(t *testing.T) {
	s, err := New(&Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	result, err := s.Run(context.Background(), "echo", "hello world")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(string(result.Stdout), "hello world") {
		t.Errorf("expected 'hello world' in output, got %s", string(result.Stdout))
	}
}

func TestSandbox_Run_CommandNotAllowed(t *testing.T) {
	s, err := New(&Config{
		Timeout:     5 * time.Second,
		AllowedCmds: []string{"echo", "ls"},
	})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	_, err = s.Run(context.Background(), "rm", "-rf", "/")
	if err == nil {
		t.Error("expected error for disallowed command")
	}

	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected 'not allowed' error, got %v", err)
	}
}

func TestSandbox_Run_Timeout(t *testing.T) {
	s, err := New(&Config{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	// Use sleep to test timeout (works on both Unix and Windows)
	result, err := s.Run(context.Background(), "sleep", "10")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !result.TimedOut {
		t.Error("expected timeout")
	}

	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for timeout")
	}
}

func TestSandbox_Run_ExitCode(t *testing.T) {
	s, err := New(&Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	result, err := s.Run(context.Background(), "sh", "-c", "exit 42")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestSandbox_RunScript(t *testing.T) {
	s, err := New(&Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	script := `#!/bin/sh
echo "Hello from script"
`

	result, err := s.RunScript(context.Background(), script, "sh")
	if err != nil {
		t.Fatalf("RunScript failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(string(result.Stdout), "Hello from script") {
		t.Errorf("expected script output, got %s", string(result.Stdout))
	}
}

func TestSandbox_RunCode_Python(t *testing.T) {
	// Skip if python3 is not available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	s, err := New(&Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	code := `print("Hello from Python")`

	result, err := s.RunCode(context.Background(), code, "python3")
	if err != nil {
		t.Fatalf("RunCode failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(string(result.Stdout), "Hello from Python") {
		t.Errorf("expected Python output, got %s", string(result.Stdout))
	}
}

func TestSandbox_RunCode_Bash(t *testing.T) {
	s, err := New(&Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	code := `echo "Hello from Bash"`

	result, err := s.RunCode(context.Background(), code, "bash")
	if err != nil {
		t.Fatalf("RunCode failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(string(result.Stdout), "Hello from Bash") {
		t.Errorf("expected Bash output, got %s", string(result.Stdout))
	}
}

func TestSandbox_RunCode_UnsupportedLanguage(t *testing.T) {
	s, err := New(&Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	_, err = s.RunCode(context.Background(), "code", "unsupported")
	if err == nil {
		t.Error("expected error for unsupported language")
	}
}

func TestSandbox_WriteFile(t *testing.T) {
	s, err := New(&Config{
		Timeout:    5 * time.Second,
		FileAccess: FileAccessWorkDirOnly,
	})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	err = s.WriteFile("test.txt", []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify file exists in work dir
	content, err := s.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(content) != "hello" {
		t.Errorf("expected 'hello', got %s", string(content))
	}
}

func TestSandbox_WriteFile_NoAccess(t *testing.T) {
	s, err := New(&Config{
		Timeout:    5 * time.Second,
		FileAccess: FileAccessNone,
	})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	err = s.WriteFile("test.txt", []byte("hello"), 0644)
	if err == nil {
		t.Error("expected error for file access denied")
	}
}

func TestSandbox_ReadFile(t *testing.T) {
	s, err := New(&Config{
		Timeout:    5 * time.Second,
		FileAccess: FileAccessWorkDirOnly,
	})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	// Create file first
	os.WriteFile(filepath.Join(s.config.WorkDir, "test.txt"), []byte("test content"), 0644)

	content, err := s.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("expected 'test content', got %s", string(content))
	}
}

func TestSandbox_ListFiles(t *testing.T) {
	s, err := New(&Config{
		Timeout:    5 * time.Second,
		FileAccess: FileAccessWorkDirOnly,
	})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	// Create some files
	os.WriteFile(filepath.Join(s.config.WorkDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(s.config.WorkDir, "b.txt"), []byte("b"), 0644)

	files, err := s.ListFiles(".")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) < 2 {
		t.Errorf("expected at least 2 files, got %d", len(files))
	}
}

func TestSandbox_GetWorkDir(t *testing.T) {
	tmpDir := t.TempDir()

	s, err := New(&Config{WorkDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	if s.GetWorkDir() != tmpDir {
		t.Errorf("expected work dir %s, got %s", tmpDir, s.GetWorkDir())
	}
}

func TestSandbox_GetTempDir(t *testing.T) {
	s, err := New(&Config{})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	tempDir := s.GetTempDir()
	if tempDir == "" {
		t.Error("expected temp dir")
	}

	// Verify temp dir exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("expected temp dir to exist")
	}
}

func TestSandbox_Cleanup(t *testing.T) {
	s, err := New(&Config{})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tempDir := s.config.TempDir

	err = s.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify temp dir is removed
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("expected temp dir to be removed")
	}
}

func TestSandbox_RuntimeInfo(t *testing.T) {
	info := GetRuntimeInfo()

	if info.OS != runtime.GOOS {
		t.Errorf("expected OS %s, got %s", runtime.GOOS, info.OS)
	}

	if info.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %s, got %s", runtime.GOARCH, info.Arch)
	}

	if info.NumCPU <= 0 {
		t.Error("expected positive NumCPU")
	}
}

func TestSandbox_FilterEnv(t *testing.T) {
	s, err := New(&Config{
		EnvWhitelist: []string{"PATH", "HOME"},
	})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	env := s.filterEnv()

	// Should contain at least PATH
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected PATH in environment")
	}
}

func TestSandbox_EmptyEnvWhitelist(t *testing.T) {
	s, err := New(&Config{})
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}
	defer s.Cleanup()

	env := s.filterEnv()

	// Should have minimal safe environment
	if len(env) == 0 {
		t.Error("expected minimal environment")
	}
}

func TestSandbox_IsPathAllowed(t *testing.T) {
	tests := []struct {
		name     string
		mode     FileAccessMode
		workDir  string
		tempDir  string
		path     string
		expected bool
	}{
		{"None mode", FileAccessNone, "/workdir", "/tempdir", "/tmp/test", false},
		{"WorkDirOnly mode - inside", FileAccessWorkDirOnly, "/workdir", "/tempdir", "/workdir/test", true},
		{"WorkDirOnly mode - outside", FileAccessWorkDirOnly, "/workdir", "/tempdir", "/etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Sandbox{
				config: &Config{
					FileAccess: tt.mode,
					WorkDir:    tt.workDir,
					TempDir:    tt.tempDir,
				},
				workDir:   tt.workDir,
				tempDir:   tt.tempDir,
			}

			result := s.isPathAllowed(tt.path)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
