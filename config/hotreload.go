// This file provides hot-reload functionality for configuration management.
// The main Config types are defined in config.go.

package config

import (
	"log/slog"
	"os"
	"sync"
	"time"
)

// HotReloader watches for configuration changes and reloads automatically.
type HotReloader struct {
	mu         sync.RWMutex
	config     *Config
	path       string
	interval   time.Duration
	stopCh     chan struct{}
	onChange   func(*Config)
	lastMod    time.Time
	fileExists bool
}

// NewHotReloader creates a new configuration hot-reloader.
func NewHotReloader(path string, interval time.Duration) *HotReloader {
	return &HotReloader{
		path:     path,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// SetOnChange sets the callback function for configuration changes.
func (hr *HotReloader) SetOnChange(fn func(*Config)) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.onChange = fn
}

// Start begins watching for configuration changes.
func (hr *HotReloader) Start() error {
	// Load initial configuration
	cfg, err := Load(hr.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet
			hr.fileExists = false
			return nil // Not an error, just no config yet
		}
		return err
	}

	hr.mu.Lock()
	hr.config = cfg
	hr.fileExists = true
	if info, err := os.Stat(hr.path); err == nil {
		hr.lastMod = info.ModTime()
	}
	hr.mu.Unlock()

	go hr.watchLoop()
	return nil
}

// Stop stops watching for changes.
func (hr *HotReloader) Stop() {
	close(hr.stopCh)
}

// GetConfig returns the current configuration.
func (hr *HotReloader) GetConfig() *Config {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.config
}

// Reload forces a configuration reload.
func (hr *HotReloader) Reload() error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	cfg, err := Load(hr.path)
	if err != nil {
		return err
	}

	hr.config = cfg
	hr.fileExists = true

	if info, err := os.Stat(hr.path); err == nil {
		hr.lastMod = info.ModTime()
	}

	if hr.onChange != nil {
		hr.onChange(cfg)
	}

	slog.Info("configuration reloaded", "path", hr.path)
	return nil
}

// watchLoop periodically checks for configuration changes.
func (hr *HotReloader) watchLoop() {
	ticker := time.NewTicker(hr.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hr.checkForChanges()
		case <-hr.stopCh:
			return
		}
	}
}

// checkForChanges checks if the configuration file has been modified.
func (hr *HotReloader) checkForChanges() {
	info, err := os.Stat(hr.path)
	if err != nil {
		if os.IsNotExist(err) {
			if hr.fileExists {
				slog.Warn("configuration file removed")
				hr.fileExists = false
			}
			return
		}
		slog.Error("failed to stat config file", "error", err)
		return
	}

	hr.mu.Lock()
	defer hr.mu.Unlock()

	modTime := info.ModTime()
	if modTime.After(hr.lastMod) {
		slog.Info("configuration file changed, reloading...")

		cfg, err := Load(hr.path)
		if err != nil {
			slog.Error("failed to reload configuration", "error", err)
			return
		}

		hr.config = cfg
		hr.lastMod = modTime
		hr.fileExists = true

		if hr.onChange != nil {
			// Call callback without holding the lock
			hr.mu.Unlock()
			hr.onChange(cfg)
			hr.mu.Lock()
		}

		slog.Info("configuration reloaded", "path", hr.path)
	}
}

// UpdateProjectProviders updates providers for a project atomically.
func (c *Config) UpdateProjectProviders(projectName string, providers []ProviderConfig) error {
	for i := range c.Projects {
		if c.Projects[i].Name == projectName {
			c.Projects[i].Agent.Providers = providers
			return saveConfig(c)
		}
	}
	return nil
}

// SetProjectAgentOption sets an agent option for a project.
func (c *Config) SetProjectAgentOption(projectName, key string, value any) error {
	for i := range c.Projects {
		if c.Projects[i].Name == projectName {
			if c.Projects[i].Agent.Options == nil {
				c.Projects[i].Agent.Options = make(map[string]any)
			}
			c.Projects[i].Agent.Options[key] = value
			return saveConfig(c)
		}
	}
	return nil
}
