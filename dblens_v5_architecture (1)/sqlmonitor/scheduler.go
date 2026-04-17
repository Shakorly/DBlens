package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Scheduler is the central execution controller for one SQL Server instance.
// It owns the worker pool, load signal, circuit breaker, and slow-cycle timers.
// The main polling loop calls RunCycle() once per interval; Scheduler decides
// exactly which tasks run, in what order, with what concurrency.
type Scheduler struct {
	server      ServerConfig
	cfg         *Config
	logger      *Logger
	pool        *WorkerPool
	circuit     *CircuitBreaker
	load        *LoadSignal
	slowState   map[string]time.Time // taskName → last successful run time
	onceRan     map[string]bool      // TierOnce tasks that already ran
	mu          sync.Mutex
	cycleCount  int
}

// NewScheduler builds a Scheduler for one server.
// maxConcurrent controls how many SQL queries can run simultaneously.
func NewScheduler(server ServerConfig, cfg *Config, logger *Logger) *Scheduler {
	// Conservative pool: max 3 simultaneous queries per server.
	// This keeps monitoring overhead negligible even on loaded servers.
	pool := NewWorkerPool(3, 8*time.Second)
	circuit := NewCircuitBreaker(
		2,              // open after 2 consecutive failures
		1*time.Minute,  // first backoff: 1 minute
		32*time.Minute, // cap backoff at 32 minutes
	)
	return &Scheduler{
		server:    server,
		cfg:       cfg,
		logger:    logger,
		pool:      pool,
		circuit:   circuit,
		load:      &LoadSignal{},
		slowState: make(map[string]time.Time),
		onceRan:   make(map[string]bool),
	}
}

// RunCycle executes one monitoring cycle for this server.
// It is safe to call concurrently from the main polling goroutine.
func (s *Scheduler) RunCycle(db *sql.DB, dispatchers map[string]Dispatcher) *CollectionResult {
	result := &CollectionResult{ServerName: s.server.Name, Timestamp: time.Now()}

	// ── Step 1: Circuit breaker check ────────────────────────────────────────
	allowed, reason := s.circuit.Allow()
	if !allowed {
		result.Errors = append(result.Errors, reason)
		return result
	}

	// ── Step 2: Probe-connect with budget ────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	if db == nil {
		result.Errors = append(result.Errors, "no database connection")
		return result
	}

	// ── Step 3: Refresh load signal (always fast — < 50ms) ───────────────────
	s.load.Refresh(ctx, db)
	stressLevel := s.load.Level()

	s.mu.Lock()
	s.cycleCount++
	cycle := s.cycleCount
	s.mu.Unlock()

	if stressLevel >= StressHigh {
		s.logger.Warn(s.server.Name, fmt.Sprintf(
			"Cycle %d | stress=%s | MEDIUM+SLOW collectors suppressed",
			cycle, stressLevel))
	} else {
		cpu, sess, _, blocked := s.load.Snapshot()
		s.logger.Debug(s.server.Name, fmt.Sprintf(
			"Cycle %d | stress=%s | cpu=%d%% sessions=%d blocked=%d",
			cycle, stressLevel, cpu, sess, blocked))
	}

	// ── Step 4: Select tasks for this cycle ────────────────────────────────────
	now := time.Now()
	tasks := AllTasks()
	var selected []Task

	for _, t := range tasks {
		switch t.Tier {

		case TierOnce:
			s.mu.Lock()
			already := s.onceRan[t.Name]
			s.mu.Unlock()
			if already {
				continue
			}
			selected = append(selected, t)

		case TierFast:
			selected = append(selected, t) // always

		case TierMedium:
			if s.load.ShouldSkip(TierMedium) {
				s.logger.Debug(s.server.Name, fmt.Sprintf("Skipping MEDIUM task [%s] (stress=%s)", t.Name, stressLevel))
				continue
			}
			selected = append(selected, t)

		case TierSlow:
			if s.load.ShouldSkip(TierSlow) {
				continue
			}
			interval := t.Interval
			if interval == 0 {
				interval = time.Duration(s.cfg.PollIntervalSeconds) * time.Second
			}
			s.mu.Lock()
			lastRan := s.slowState[t.Name]
			s.mu.Unlock()
			if now.Sub(lastRan) >= interval {
				selected = append(selected, t)
			}
		}
	}

	// ── Step 5: Run selected tasks via worker pool ────────────────────────────
	// FAST tasks run sequentially (they're cheap and order matters for some).
	// MEDIUM and SLOW tasks run concurrently via the pool (bounded by 3 slots).

	var wg sync.WaitGroup
	var mu sync.Mutex

	runTask := func(t Task) {
		defer wg.Done()

		taskCtx, taskCancel := context.WithTimeout(ctx, t.QueryBudget)
		defer taskCancel()

		dispatch, ok := dispatchers[t.Name]
		if !ok {
			return
		}

		start := time.Now()
		if err := dispatch(taskCtx, db, s.server.Name, s.cfg, result, &mu); err != nil {
			mu.Lock()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", t.Name, err))
			mu.Unlock()
			s.logger.Debug(s.server.Name, fmt.Sprintf("Task [%s] failed in %v: %v",
				t.Name, time.Since(start).Round(time.Millisecond), err))
			return
		}

		elapsed := time.Since(start).Round(time.Millisecond)
		if elapsed > t.QueryBudget*80/100 {
			s.logger.Warn(s.server.Name, fmt.Sprintf(
				"Task [%s] used %.0f%% of budget (%v of %v)",
				t.Name, float64(elapsed)/float64(t.QueryBudget)*100, elapsed, t.QueryBudget))
		}

		// Mark slow/once tasks as completed
		if t.Tier == TierSlow {
			s.mu.Lock()
			s.slowState[t.Name] = now
			s.mu.Unlock()
		}
		if t.Tier == TierOnce {
			s.mu.Lock()
			s.onceRan[t.Name] = true
			s.mu.Unlock()
		}
	}

	// Partition selected tasks by tier
	var fastTasks, asyncTasks []Task
	for _, t := range selected {
		if t.Tier == TierFast {
			fastTasks = append(fastTasks, t)
		} else {
			asyncTasks = append(asyncTasks, t)
		}
	}

	// Run FAST tasks sequentially first (no pool needed — they're cheap)
	for _, t := range fastTasks {
		wg.Add(1)
		runTask(t)
	}

	// Run MEDIUM/SLOW/ONCE tasks via the bounded worker pool
	for _, t := range asyncTasks {
		wg.Add(1)
		t := t
		acquired := s.pool.Run(ctx, func() { runTask(t) })
		if !acquired {
			wg.Done() // pool full/timeout — skip this task this cycle
			s.logger.Warn(s.server.Name, fmt.Sprintf("Worker pool full — skipping task [%s] this cycle", t.Name))
		}
	}

	wg.Wait()
	return result
}

// Dispatcher is the function signature the scheduler calls for each named task.
// Each collector registers itself in the dispatcher table in dispatchers.go.
type Dispatcher func(
	ctx context.Context,
	db *sql.DB,
	serverName string,
	cfg *Config,
	result *CollectionResult,
	mu *sync.Mutex,
) error
