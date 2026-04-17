package main

import (
	"context"
	"sync"
	"time"
)

// WorkerPool limits concurrent SQL queries against a single server to avoid
// overwhelming it with monitoring traffic. This is a semaphore-backed pool
// where each "slot" represents permission to run one query at a time.
type WorkerPool struct {
	sem     chan struct{}
	timeout time.Duration
	mu      sync.Mutex
	// Metrics
	acquired  int
	rejected  int
	maxQueued int
}

// NewWorkerPool creates a pool with the given concurrency limit.
// timeout is the max time a task will wait to acquire a slot before giving up.
func NewWorkerPool(maxConcurrent int, acquireTimeout time.Duration) *WorkerPool {
	return &WorkerPool{
		sem:     make(chan struct{}, maxConcurrent),
		timeout: acquireTimeout,
	}
}

// Run acquires a slot, executes fn, then releases the slot.
// Returns false if the slot could not be acquired within the timeout.
func (wp *WorkerPool) Run(ctx context.Context, fn func()) bool {
	// Try to acquire a slot
	select {
	case wp.sem <- struct{}{}:
		// Got a slot immediately
	case <-time.After(wp.timeout):
		wp.mu.Lock()
		wp.rejected++
		wp.mu.Unlock()
		return false
	case <-ctx.Done():
		return false
	}

	wp.mu.Lock()
	wp.acquired++
	if len(wp.sem) > wp.maxQueued {
		wp.maxQueued = len(wp.sem)
	}
	wp.mu.Unlock()

	go func() {
		defer func() { <-wp.sem }() // release slot when done
		fn()
	}()
	return true
}

// Stats returns pool utilisation stats for monitoring/logging.
func (wp *WorkerPool) Stats() (capacity, inUse, acquired, rejected int) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return cap(wp.sem), len(wp.sem), wp.acquired, wp.rejected
}

// ── Per-server connection circuit breaker ─────────────────────────────────────

// CircuitBreaker prevents hammering an unreachable server.
// States: Closed (normal) → Open (failing, backoff) → HalfOpen (testing)
type CircuitBreaker struct {
	mu           sync.Mutex
	failures     int
	openUntil    time.Time
	maxFailures  int
	baseBackoff  time.Duration
	maxBackoff   time.Duration
}

func NewCircuitBreaker(maxFailures int, baseBackoff, maxBackoff time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: maxFailures,
		baseBackoff: baseBackoff,
		maxBackoff:  maxBackoff,
	}
}

// Allow returns (true, "") if the circuit is closed/half-open and a request may proceed.
// Returns (false, reason) if the circuit is open.
func (cb *CircuitBreaker) Allow() (bool, string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now()
	if cb.failures == 0 {
		return true, ""
	}
	if now.Before(cb.openUntil) {
		return false, "skipped — circuit open until " +
			cb.openUntil.Format("15:04:05") +
			" (" + itoa(cb.failures) + " consecutive fails)"
	}
	// Half-open: let one request through
	return true, ""
}

// RecordSuccess resets the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.openUntil = time.Time{}
}

// RecordFailure increments the failure count and opens the circuit with exponential backoff.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	backoff := cb.baseBackoff * (1 << min(cb.failures-1, 5)) // 1m, 2m, 4m, 8m, 16m, 32m max
	if backoff > cb.maxBackoff {
		backoff = cb.maxBackoff
	}
	cb.openUntil = time.Now().Add(backoff)
}

// Failures returns the current consecutive failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}

func min(a, b int) int {
	if a < b { return a }
	return b
}

func itoa(n int) string {
	if n == 0 { return "0" }
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
