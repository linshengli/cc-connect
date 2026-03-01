package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHotReloader_Start(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Create initial config
	content := `
[[projects]]
name = "test"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	os.WriteFile(configPath, []byte(content), 0644)

	hr := NewHotReloader(configPath, 100*time.Millisecond)

	err := hr.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer hr.Stop()

	cfg := hr.GetConfig()
	if cfg == nil {
		t.Fatal("expected configuration")
	}
}

func TestHotReloader_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	content := `
language = "en"

[[projects]]
name = "test"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	os.WriteFile(configPath, []byte(content), 0644)

	hr := NewHotReloader(configPath, time.Second)
	hr.Start()
	defer hr.Stop()

	// Modify file
	newContent := `
language = "zh"

[[projects]]
name = "test"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	time.Sleep(10 * time.Millisecond) // Ensure different mod time
	os.WriteFile(configPath, []byte(newContent), 0644)

	// Force reload
	err := hr.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	cfg := hr.GetConfig()
	if cfg.Language != "zh" {
		t.Errorf("expected language 'zh', got %q", cfg.Language)
	}
}

func TestHotReloader_OnChange(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	content := `
language = "en"

[[projects]]
name = "test"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	os.WriteFile(configPath, []byte(content), 0644)

	hr := NewHotReloader(configPath, 50*time.Millisecond)

	called := false
	hr.SetOnChange(func(cfg *Config) {
		called = true
		if cfg.Language != "zh" {
			t.Errorf("expected language 'zh' in callback, got %q", cfg.Language)
		}
	})

	hr.Start()
	defer hr.Stop()

	// Modify file
	newContent := `
language = "zh"

[[projects]]
name = "test"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(configPath, []byte(newContent), 0644)

	// Wait for detection
	time.Sleep(200 * time.Millisecond)

	if !called {
		t.Error("expected OnChange callback to be called")
	}
}

func TestConfig_UpdateProjectProviders(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	ConfigPath = configPath

	content := `
[[projects]]
name = "test-project"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	providers := []ProviderConfig{
		{Name: "provider1", APIKey: "key1"},
		{Name: "provider2", APIKey: "key2"},
	}

	err = cfg.UpdateProjectProviders("test-project", providers)
	if err != nil {
		t.Fatalf("UpdateProjectProviders failed: %v", err)
	}

	// Verify by reloading from file
	reloaded, _ := Load(configPath)
	if len(reloaded.Projects[0].Agent.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(reloaded.Projects[0].Agent.Providers))
	}
}

func TestConfig_SetProjectAgentOption(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	ConfigPath = configPath

	content := `
[[projects]]
name = "test-project"
[projects.agent]
type = "claudecode"
[projects.agent.options]
work_dir = "/tmp"
[[projects.platforms]]
type = "feishu"
[projects.platforms.options]
app_id = "test"
app_secret = "test"
`
	os.WriteFile(configPath, []byte(content), 0644)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err = cfg.SetProjectAgentOption("test-project", "mode", "yolo")
	if err != nil {
		t.Fatalf("SetProjectAgentOption failed: %v", err)
	}

	// Verify by reloading from file
	reloaded, _ := Load(configPath)
	mode, ok := reloaded.Projects[0].Agent.Options["mode"].(string)
	if !ok || mode != "yolo" {
		t.Errorf("expected mode 'yolo', got %v", mode)
	}
}

func TestHotReloader_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.toml")

	hr := NewHotReloader(configPath, 100*time.Millisecond)

	// Start should handle non-existent file gracefully
	err := hr.Start()
	// For non-existent file, it should return nil (file doesn't exist yet is OK)
	// The error might be wrapped, so check for the specific message
	if err != nil {
		if !containsString(err.Error(), "no such file") && !os.IsNotExist(err) {
			t.Fatalf("Start failed with unexpected error: %v", err)
		}
		// If error is about file not existing, that's acceptable
	}
	defer hr.Stop()

	// Config should be nil initially
	cfg := hr.GetConfig()
	if cfg != nil {
		t.Error("expected nil config for non-existent file")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
