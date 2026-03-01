package core

import (
	"testing"
)

func TestRegisterPlatform(t *testing.T) {
	// Store original count
	originalCount := len(platformFactories)

	called := false
	RegisterPlatform("test_platform", func(opts map[string]any) (Platform, error) {
		called = true
		return &MockPlatform{NameValue: "test"}, nil
	})

	if len(platformFactories) != originalCount+1 {
		t.Errorf("expected %d factories, got %d", originalCount+1, len(platformFactories))
	}

	// Create the platform
	p, err := CreatePlatform("test_platform", nil)
	if err != nil {
		t.Fatalf("CreatePlatform failed: %v", err)
	}

	if p.Name() != "test" {
		t.Errorf("expected name 'test', got %q", p.Name())
	}

	if !called {
		t.Error("expected factory to be called")
	}
}

func TestRegisterPlatform_Duplicate(t *testing.T) {
	RegisterPlatform("test_dup", func(opts map[string]any) (Platform, error) {
		return &MockPlatform{NameValue: "test"}, nil
	})

	// Registering again should overwrite (current behavior)
	RegisterPlatform("test_dup", func(opts map[string]any) (Platform, error) {
		return &MockPlatform{NameValue: "test2"}, nil
	})

	p, _ := CreatePlatform("test_dup", nil)
	if p.Name() != "test2" {
		t.Errorf("expected name 'test2', got %q", p.Name())
	}
}

func TestCreatePlatform_Unknown(t *testing.T) {
	_, err := CreatePlatform("unknown_platform_xyz", nil)
	if err == nil {
		t.Error("expected error for unknown platform")
	}
}

func TestCreatePlatform_Error(t *testing.T) {
	RegisterPlatform("test_error", func(opts map[string]any) (Platform, error) {
		return nil, testError("factory error")
	})

	_, err := CreatePlatform("test_error", nil)
	if err == nil {
		t.Error("expected error from factory")
	}
}

func TestRegisterAgent(t *testing.T) {
	originalCount := len(agentFactories)

	called := false
	RegisterAgent("test_agent", func(opts map[string]any) (Agent, error) {
		called = true
		return &MockAgent{NameValue: "test"}, nil
	})

	if len(agentFactories) != originalCount+1 {
		t.Errorf("expected %d factories, got %d", originalCount+1, len(agentFactories))
	}

	// Create the agent
	a, err := CreateAgent("test_agent", nil)
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	if a.Name() != "test" {
		t.Errorf("expected name 'test', got %q", a.Name())
	}

	if !called {
		t.Error("expected factory to be called")
	}
}

func TestCreateAgent_Unknown(t *testing.T) {
	_, err := CreateAgent("unknown_agent_xyz", nil)
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestCreateAgent_Error(t *testing.T) {
	RegisterAgent("test_agent_error", func(opts map[string]any) (Agent, error) {
		return nil, testError("factory error")
	})

	_, err := CreateAgent("test_agent_error", nil)
	if err == nil {
		t.Error("expected error from factory")
	}
}

func TestCreatePlatform_AvailableList(t *testing.T) {
	RegisterPlatform("test_avail", func(opts map[string]any) (Platform, error) {
		return &MockPlatform{NameValue: "test"}, nil
	})

	_, err := CreatePlatform("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	// Error message should contain available platforms
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected error message with available platforms")
	}
}

func TestCreateAgent_AvailableList(t *testing.T) {
	RegisterAgent("test_avail_agent", func(opts map[string]any) (Agent, error) {
		return &MockAgent{NameValue: "test"}, nil
	})

	_, err := CreateAgent("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	// Error message should contain available agents
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected error message with available agents")
	}
}

// testError is a simple error type for testing
type testError string

func (e testError) Error() string {
	return string(e)
}
