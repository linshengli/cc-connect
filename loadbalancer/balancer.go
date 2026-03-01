// Package loadbalancer provides load balancing and failover for API providers.
package loadbalancer

import (
	"errors"
	"sync"
	"time"
)

// ProviderState represents the health state of a provider.
type ProviderState int

const (
	StateHealthy ProviderState = iota
	StateDegraded
	StateUnhealthy
)

// Provider represents an API provider with health tracking.
type Provider struct {
	Name           string
	BaseURL        string
	APIKey         string
	Model          string
	Env            map[string]string
	State          ProviderState
	LastError      error
	LastCheckAt    time.Time
	ConsecutiveErr int
	TotalRequests  int64
	TotalErrors    int64
	Weight         int // for weighted round-robin
	Priority       int // lower = higher priority
}

// Strategy defines the load balancing strategy.
type Strategy string

const (
	StrategyRoundRobin    Strategy = "round_robin"
	StrategyWeightedRR    Strategy = "weighted_round_robin"
	StrategyPriority      Strategy = "priority"
	StrategyLeastErrors   Strategy = "least_errors"
	StrategyRandom        Strategy = "random"
)

// Balancer manages provider selection and failover.
type Balancer struct {
	mu             sync.RWMutex
	providers      map[string]*Provider
	providerList   []*Provider
	strategy       Strategy
	currentIdx     int
	healthCheckFn  HealthCheckFunc
	autoFailover   bool
	maxErrors      int // consecutive errors before failover
	healthCheckInt time.Duration
	stopCh         chan struct{}
}

// HealthCheckFunc checks if a provider is healthy.
type HealthCheckFunc func(provider *Provider) (bool, error)

// NewBalancer creates a new load balancer.
func NewBalancer(strategy Strategy) *Balancer {
	return &Balancer{
		providers:      make(map[string]*Provider),
		providerList:   make([]*Provider, 0),
		strategy:       strategy,
		autoFailover:   true,
		maxErrors:      3,
		healthCheckInt: 30 * time.Second,
		stopCh:         make(chan struct{}),
	}
}

// AddProvider adds a provider to the balancer.
func (b *Balancer) AddProvider(p *Provider) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p.Weight == 0 {
		p.Weight = 1
	}

	b.providers[p.Name] = p
	b.providerList = append(b.providerList, p)
}

// RemoveProvider removes a provider by name.
func (b *Balancer) RemoveProvider(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.providers, name)

	newList := make([]*Provider, 0, len(b.providerList))
	for _, p := range b.providerList {
		if p.Name != name {
			newList = append(newList, p)
		}
	}
	b.providerList = newList
}

// GetProvider returns the next available provider based on the strategy.
func (b *Balancer) GetProvider() (*Provider, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.providerList) == 0 {
		return nil, errors.New("no providers configured")
	}

	// Try to get a healthy provider based on strategy
	provider, err := b.selectProvider()

	// Update stats
	if provider != nil {
		provider.TotalRequests++
		provider.LastCheckAt = time.Now()
	}

	return provider, err
}

// selectProvider selects a provider based on the configured strategy.
func (b *Balancer) selectProvider() (*Provider, error) {
	switch b.strategy {
	case StrategyRoundRobin:
		return b.roundRobinSelect()
	case StrategyWeightedRR:
		return b.weightedRoundRobinSelect()
	case StrategyPriority:
		return b.prioritySelect()
	case StrategyLeastErrors:
		return b.leastErrorsSelect()
	case StrategyRandom:
		return b.randomSelect()
	default:
		return b.roundRobinSelect()
	}
}

// roundRobinSelect selects providers in round-robin fashion.
func (b *Balancer) roundRobinSelect() (*Provider, error) {
	startIdx := b.currentIdx
	tried := 0

	for tried < len(b.providerList) {
		provider := b.providerList[b.currentIdx]
		b.currentIdx = (b.currentIdx + 1) % len(b.providerList)

		if provider.State != StateUnhealthy {
			return provider, nil
		}
		tried++
	}

	// All providers unhealthy, return first one anyway
	return b.providerList[startIdx%len(b.providerList)], errors.New("all providers unhealthy")
}

// weightedRoundRobinSelect selects based on weights.
func (b *Balancer) weightedRoundRobinSelect() (*Provider, error) {
	// Simple weighted selection - prefer higher weight providers
	totalWeight := 0
	for _, p := range b.providerList {
		if p.State != StateUnhealthy {
			totalWeight += p.Weight
		}
	}

	if totalWeight == 0 {
		return b.providerList[0], errors.New("all providers unhealthy")
	}

	// Use current index modulo total weight for selection
	target := b.currentIdx % totalWeight
	b.currentIdx++

	current := 0
	for _, p := range b.providerList {
		if p.State != StateUnhealthy {
			current += p.Weight
			if current > target {
				return p, nil
			}
		}
	}

	return b.providerList[0], nil
}

// prioritySelect selects the highest priority healthy provider.
func (b *Balancer) prioritySelect() (*Provider, error) {
	var best *Provider

	for _, p := range b.providerList {
		if p.State == StateUnhealthy {
			continue
		}
		if best == nil || p.Priority < best.Priority {
			best = p
		}
	}

	if best == nil {
		// All unhealthy, return highest priority anyway
		for _, p := range b.providerList {
			if best == nil || p.Priority < best.Priority {
				best = p
			}
		}
	}

	return best, nil
}

// leastErrorsSelect selects the provider with fewest consecutive errors.
func (b *Balancer) leastErrorsSelect() (*Provider, error) {
	var best *Provider

	for _, p := range b.providerList {
		if p.State == StateUnhealthy {
			continue
		}
		if best == nil || p.ConsecutiveErr < best.ConsecutiveErr {
			best = p
		}
	}

	if best == nil {
		return b.providerList[0], errors.New("all providers unhealthy")
	}

	return best, nil
}

// randomSelect selects a random healthy provider.
func (b *Balancer) randomSelect() (*Provider, error) {
	// Use time-based pseudo-random selection
	now := time.Now().UnixNano()
	idx := int(now) % len(b.providerList)

	startIdx := idx
	tried := 0

	for tried < len(b.providerList) {
		provider := b.providerList[idx]
		idx = (idx + 1) % len(b.providerList)

		if provider.State != StateUnhealthy {
			return provider, nil
		}
		tried++
	}

	return b.providerList[startIdx%len(b.providerList)], errors.New("all providers unhealthy")
}

// ReportSuccess reports a successful request for a provider.
func (b *Balancer) ReportSuccess(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	provider, ok := b.providers[name]
	if !ok {
		return
	}

	provider.ConsecutiveErr = 0
	provider.LastError = nil
	if provider.State == StateDegraded {
		provider.State = StateHealthy
	}
}

// ReportError reports a failed request for a provider.
func (b *Balancer) ReportError(name string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	provider, ok := b.providers[name]
	if !ok {
		return
	}

	provider.ConsecutiveErr++
	provider.LastError = err
	provider.TotalErrors++

	// Update state based on consecutive errors
	if provider.ConsecutiveErr >= b.maxErrors {
		provider.State = StateUnhealthy
	} else if provider.ConsecutiveErr > 0 {
		provider.State = StateDegraded
	}

	// Auto-failover: if this provider was the only one and it failed,
	// the next GetProvider will automatically try others
}

// SetHealthCheck sets the health check function.
func (b *Balancer) SetHealthCheck(fn HealthCheckFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.healthCheckFn = fn
}

// SetAutoFailover enables/disables automatic failover.
func (b *Balancer) SetAutoFailover(enabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.autoFailover = enabled
}

// SetMaxErrors sets the consecutive error threshold for failover.
func (b *Balancer) SetMaxErrors(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxErrors = n
}

// StartHealthChecks starts periodic health checks.
func (b *Balancer) StartHealthChecks() {
	if b.healthCheckFn == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(b.healthCheckInt)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.performHealthChecks()
			case <-b.stopCh:
				return
			}
		}
	}()
}

// StopHealthChecks stops the health check goroutine.
func (b *Balancer) StopHealthChecks() {
	close(b.stopCh)
}

// performHealthChecks checks all providers.
func (b *Balancer) performHealthChecks() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, provider := range b.providerList {
		healthy, err := b.healthCheckFn(provider)
		if healthy {
			provider.State = StateHealthy
			provider.LastError = nil
			provider.ConsecutiveErr = 0
		} else {
			if err != nil {
				provider.LastError = err
			}
			if provider.State == StateHealthy {
				provider.State = StateDegraded
			}
		}
		provider.LastCheckAt = time.Now()
	}
}

// GetStats returns statistics for all providers.
func (b *Balancer) GetStats() []ProviderStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := make([]ProviderStats, 0, len(b.providerList))
	for _, p := range b.providerList {
		stats = append(stats, ProviderStats{
			Name:           p.Name,
			State:          p.State,
			TotalRequests:  p.TotalRequests,
			TotalErrors:    p.TotalErrors,
			ConsecutiveErr: p.ConsecutiveErr,
			Weight:         p.Weight,
			Priority:       p.Priority,
		})
	}
	return stats
}

// ProviderStats holds statistics for a provider.
type ProviderStats struct {
	Name           string
	State          ProviderState
	TotalRequests  int64
	TotalErrors    int64
	ConsecutiveErr int
	Weight         int
	Priority       int
}

// ListProviders returns all configured providers.
func (b *Balancer) ListProviders() []*Provider {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*Provider, len(b.providerList))
	copy(result, b.providerList)
	return result
}

// GetProviderByName returns a specific provider.
func (b *Balancer) GetProviderByName(name string) (*Provider, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	p, ok := b.providers[name]
	return p, ok
}

// SetStrategy changes the load balancing strategy.
func (b *Balancer) SetStrategy(strategy Strategy) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.strategy = strategy
}

// Reset resets all provider states to healthy.
func (b *Balancer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, p := range b.providerList {
		p.State = StateHealthy
		p.ConsecutiveErr = 0
		p.LastError = nil
	}
}

// StateString returns a string representation of the state.
func StateString(state ProviderState) string {
	switch state {
	case StateHealthy:
		return "healthy"
	case StateDegraded:
		return "degraded"
	case StateUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}
