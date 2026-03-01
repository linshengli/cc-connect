package loadbalancer

import (
	"errors"
	"testing"
)

func TestBalancer_New(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	if b == nil {
		t.Fatal("expected balancer")
	}

	if b.strategy != StrategyRoundRobin {
		t.Errorf("expected round_robin strategy")
	}

	if !b.autoFailover {
		t.Error("expected autoFailover enabled by default")
	}
}

func TestBalancer_AddProvider(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	p := &Provider{
		Name:     "provider1",
		BaseURL:  "https://api.example.com",
		APIKey:   "key1",
		Weight:   2,
		Priority: 1,
	}

	b.AddProvider(p)

	if len(b.providerList) != 1 {
		t.Errorf("expected 1 provider, got %d", len(b.providerList))
	}

	if b.providers["provider1"] == nil {
		t.Error("expected provider to be stored")
	}
}

func TestBalancer_RemoveProvider(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1"})
	b.AddProvider(&Provider{Name: "p2"})
	b.AddProvider(&Provider{Name: "p3"})

	b.RemoveProvider("p2")

	if len(b.providerList) != 2 {
		t.Errorf("expected 2 providers, got %d", len(b.providerList))
	}

	if _, ok := b.providers["p2"]; ok {
		t.Error("expected p2 to be removed")
	}
}

func TestBalancer_GetProvider_RoundRobin(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1"})
	b.AddProvider(&Provider{Name: "p2"})
	b.AddProvider(&Provider{Name: "p3"})

	// Should cycle through providers
	first, _ := b.GetProvider()
	second, _ := b.GetProvider()
	third, _ := b.GetProvider()
	fourth, _ := b.GetProvider()

	if first.Name != "p1" {
		t.Errorf("expected p1, got %s", first.Name)
	}
	if second.Name != "p2" {
		t.Errorf("expected p2, got %s", second.Name)
	}
	if third.Name != "p3" {
		t.Errorf("expected p3, got %s", third.Name)
	}
	if fourth.Name != "p1" {
		t.Errorf("expected p1 (cycle), got %s", fourth.Name)
	}
}

func TestBalancer_GetProvider_Priority(t *testing.T) {
	b := NewBalancer(StrategyPriority)

	b.AddProvider(&Provider{Name: "p1", Priority: 3})
	b.AddProvider(&Provider{Name: "p2", Priority: 1})
	b.AddProvider(&Provider{Name: "p3", Priority: 2})

	// Should always return lowest priority first
	for i := 0; i < 5; i++ {
		p, _ := b.GetProvider()
		if p.Name != "p2" {
			t.Errorf("expected p2 (priority 1), got %s", p.Name)
		}
	}
}

func TestBalancer_GetProvider_LeastErrors(t *testing.T) {
	b := NewBalancer(StrategyLeastErrors)

	b.AddProvider(&Provider{Name: "p1"})
	b.AddProvider(&Provider{Name: "p2"})

	// Simulate errors on p1
	b.ReportError("p1", errors.New("test error"))
	b.ReportError("p1", errors.New("test error"))

	// Should prefer p2 (fewer errors)
	for i := 0; i < 5; i++ {
		p, _ := b.GetProvider()
		if p.Name != "p2" {
			t.Errorf("expected p2 (fewer errors), got %s", p.Name)
		}
	}
}

func TestBalancer_ReportSuccess(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)
	b.maxErrors = 3

	p := &Provider{Name: "p1"}
	b.AddProvider(p)

	// Simulate errors
	b.ReportError("p1", errors.New("err1"))
	b.ReportError("p1", errors.New("err2"))

	if p.ConsecutiveErr != 2 {
		t.Errorf("expected 2 consecutive errors, got %d", p.ConsecutiveErr)
	}

	// Report success
	b.ReportSuccess("p1")

	if p.ConsecutiveErr != 0 {
		t.Errorf("expected 0 consecutive errors after success, got %d", p.ConsecutiveErr)
	}

	if p.State != StateHealthy {
		t.Errorf("expected healthy state, got %v", p.State)
	}
}

func TestBalancer_ReportError_Failover(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)
	b.maxErrors = 3

	p := &Provider{Name: "p1"}
	b.AddProvider(p)

	// Simulate errors until failover
	b.ReportError("p1", errors.New("err1"))
	if p.State != StateDegraded {
		t.Errorf("expected degraded after 1 error, got %v", p.State)
	}

	b.ReportError("p1", errors.New("err2"))
	if p.State != StateDegraded {
		t.Errorf("expected degraded after 2 errors, got %v", p.State)
	}

	b.ReportError("p1", errors.New("err3"))
	if p.State != StateUnhealthy {
		t.Errorf("expected unhealthy after 3 errors, got %v", p.State)
	}
}

func TestBalancer_GetProvider_SkipUnhealthy(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1"})
	b.AddProvider(&Provider{Name: "p2"})

	// Make p1 unhealthy
	b.ReportError("p1", errors.New("err"))
	b.ReportError("p1", errors.New("err"))
	b.ReportError("p1", errors.New("err"))

	// Should skip p1 and return p2
	p, _ := b.GetProvider()
	if p.Name != "p2" {
		t.Errorf("expected p2 (p1 unhealthy), got %s", p.Name)
	}
}

func TestBalancer_GetProvider_NoProviders(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	_, err := b.GetProvider()
	if err == nil {
		t.Error("expected error when no providers")
	}
}

func TestBalancer_GetProvider_AllUnhealthy(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1"})
	b.AddProvider(&Provider{Name: "p2"})

	// Make all unhealthy
	b.ReportError("p1", errors.New("err"))
	b.ReportError("p1", errors.New("err"))
	b.ReportError("p1", errors.New("err"))
	b.ReportError("p2", errors.New("err"))
	b.ReportError("p2", errors.New("err"))
	b.ReportError("p2", errors.New("err"))

	// Should still return a provider (first one) with error
	p, err := b.GetProvider()
	if err == nil {
		t.Error("expected error when all unhealthy")
	}
	// When all unhealthy, should return one anyway
	if p == nil {
		t.Fatal("expected a provider even when all unhealthy")
	}
}

func TestBalancer_SetStrategy(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)
	b.SetStrategy(StrategyPriority)

	if b.strategy != StrategyPriority {
		t.Errorf("expected priority strategy, got %v", b.strategy)
	}
}

func TestBalancer_SetAutoFailover(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)
	b.SetAutoFailover(false)

	if b.autoFailover {
		t.Error("expected autoFailover disabled")
	}
}

func TestBalancer_SetMaxErrors(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)
	b.SetMaxErrors(5)

	if b.maxErrors != 5 {
		t.Errorf("expected maxErrors=5, got %d", b.maxErrors)
	}
}

func TestBalancer_GetStats(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1", Weight: 2})
	b.AddProvider(&Provider{Name: "p2", Weight: 1})

	// Generate some traffic
	b.GetProvider() // p1
	b.GetProvider() // p2
	b.ReportError("p1", errors.New("test"))

	stats := b.GetStats()

	if len(stats) != 2 {
		t.Errorf("expected 2 stats, got %d", len(stats))
	}

	var p1Stats *ProviderStats
	for i := range stats {
		if stats[i].Name == "p1" {
			p1Stats = &stats[i]
		}
	}

	if p1Stats == nil {
		t.Fatal("expected p1 stats")
	}

	if p1Stats.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", p1Stats.TotalRequests)
	}

	if p1Stats.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", p1Stats.TotalErrors)
	}
}

func TestBalancer_ListProviders(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1"})
	b.AddProvider(&Provider{Name: "p2"})

	list := b.ListProviders()

	if len(list) != 2 {
		t.Errorf("expected 2 providers, got %d", len(list))
	}
}

func TestBalancer_GetProviderByName(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	b.AddProvider(&Provider{Name: "p1", Priority: 5})

	p, ok := b.GetProviderByName("p1")
	if !ok {
		t.Error("expected to find p1")
	}
	if p.Priority != 5 {
		t.Errorf("expected priority 5, got %d", p.Priority)
	}

	_, ok = b.GetProviderByName("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent")
	}
}

func TestBalancer_Reset(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)
	b.maxErrors = 3

	b.AddProvider(&Provider{Name: "p1"})

	// Make unhealthy
	for i := 0; i < 3; i++ {
		b.ReportError("p1", errors.New("err"))
	}

	if b.providers["p1"].State != StateUnhealthy {
		t.Fatal("expected unhealthy")
	}

	// Reset
	b.Reset()

	if b.providers["p1"].State != StateHealthy {
		t.Errorf("expected healthy after reset, got %v", b.providers["p1"].State)
	}

	if b.providers["p1"].ConsecutiveErr != 0 {
		t.Errorf("expected 0 errors after reset")
	}
}

func TestBalancer_WeightedRoundRobin(t *testing.T) {
	b := NewBalancer(StrategyWeightedRR)

	b.AddProvider(&Provider{Name: "p1", Weight: 3})
	b.AddProvider(&Provider{Name: "p2", Weight: 1})

	// p1 should be selected more often due to higher weight
	p1Count := 0
	for i := 0; i < 100; i++ {
		p, _ := b.GetProvider()
		if p.Name == "p1" {
			p1Count++
		}
	}

	// p1 should have roughly 75% of selections (3:1 ratio)
	if p1Count < 60 || p1Count > 90 {
		t.Errorf("expected p1 to be selected ~75%% of time, got %d/100", p1Count)
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    ProviderState
		expected string
	}{
		{StateHealthy, "healthy"},
		{StateDegraded, "degraded"},
		{StateUnhealthy, "unhealthy"},
		{ProviderState(99), "unknown"},
	}

	for _, test := range tests {
		result := StateString(test.state)
		if result != test.expected {
			t.Errorf("StateString(%v) = %s, expected %s", test.state, result, test.expected)
		}
	}
}

func TestBalancer_HealthCheck(t *testing.T) {
	b := NewBalancer(StrategyRoundRobin)

	checkCalled := false
	b.SetHealthCheck(func(p *Provider) (bool, error) {
		checkCalled = true
		return true, nil
	})

	b.AddProvider(&Provider{Name: "p1"})

	// Manually trigger health check
	b.performHealthChecks()

	if !checkCalled {
		t.Error("expected health check to be called")
	}
}
