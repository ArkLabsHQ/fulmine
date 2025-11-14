package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// TaskState describes the lifecycle state of a monitored goroutine.
type TaskState string

const (
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCanceled  TaskState = "canceled"
	TaskStatePanicked  TaskState = "panicked"
)

// TaskStatus captures the latest snapshot for a monitored goroutine.
type TaskStatus struct {
	Name             string    `json:"name"`
	State            TaskState `json:"state"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	LastHeartbeat    time.Time `json:"last_heartbeat"`
	Error            string    `json:"error"`
	Panic            string    `json:"panic"`
	HeartbeatStalled bool      `json:"heartbeat_stalled"`
}

// MonitorStatus aggregates the current status of every task.
type MonitorStatus struct {
	StartedAt time.Time    `json:"started_at"`
	Tasks     []TaskStatus `json:"tasks"`
}

// TaskFunc defines the function signature executed under Monitor.Go.
type TaskFunc func(ctx context.Context, hb Heartbeat) error

// Heartbeat is used by monitored goroutines to notify the monitor they are still alive.
type Heartbeat interface {
	Tick()
}

// Monitor supervises long-running goroutines, tracking heartbeats and surfacing crashes.
type Monitor struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.RWMutex
	tasks map[string]*taskRecord
	wg    sync.WaitGroup

	seq uint64

	stallThreshold time.Duration
	checkInterval  time.Duration
	logger         Logger
	startedAt      time.Time
}

// Logger is the subset used by the monitor for structured messages.
type Logger interface {
	Printf(format string, args ...any)
}

// Option customizes a Monitor.
type Option func(*Monitor)

// WithStallThreshold overrides the duration allowed between heartbeats before a warning is raised.
func WithStallThreshold(d time.Duration) Option {
	return func(m *Monitor) {
		m.stallThreshold = d
	}
}

// WithCheckInterval adjusts how often the watchdog inspects tasks.
func WithCheckInterval(d time.Duration) Option {
	return func(m *Monitor) {
		m.checkInterval = d
	}
}

// WithLogger uses the provided logger instead of the default stdlib logger.
func WithLogger(l Logger) Option {
	return func(m *Monitor) {
		if l != nil {
			m.logger = l
		}
	}
}

// New creates a Monitor with reasonable defaults.
func New(opts ...Option) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Monitor{
		ctx:            ctx,
		cancel:         cancel,
		tasks:          make(map[string]*taskRecord),
		stallThreshold: 2 * time.Minute,
		checkInterval:  5 * time.Second,
		logger:         log.Default(),
		startedAt:      time.Now(),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.checkInterval > 0 && m.stallThreshold > 0 {
		go m.watchdog()
	}
	return m
}

// TaskHandle exposes limited control over a monitored goroutine.
type TaskHandle struct {
	Name   string
	cancel context.CancelFunc
	done   chan struct{}
	mon    *Monitor
}

// Stop signal cancels the goroutine context.
func (h TaskHandle) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

// Done is closed when the goroutine exits.
func (h TaskHandle) Done() <-chan struct{} {
	return h.done
}

// Status reads the latest status for the task.
func (h TaskHandle) Status() TaskStatus {
	return h.mon.taskStatus(h.Name)
}

// Go runs fn in its own goroutine, tracking heartbeats and state transitions.
func (m *Monitor) Go(name string, fn TaskFunc) TaskHandle {
	if name == "" {
		name = fmt.Sprintf("task-%d", atomic.AddUint64(&m.seq, 1))
	}
	taskCtx, cancel := context.WithCancel(m.ctx)
	record := newTaskRecord(name)
	m.mu.Lock()
	m.tasks[name] = record
	m.mu.Unlock()

	done := make(chan struct{})
	hb := &heartbeat{task: record}

	m.wg.Add(1)
	go func() {
		defer close(done)
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				record.markPanicked(fmt.Sprint(r))
				m.logger.Printf("monitor: task %s panicked: %v", name, r)
			}
			cancel()
		}()

		if err := fn(taskCtx, hb); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				record.markCanceled(err)
			} else {
				record.markFailed(err)
				m.logger.Printf("monitor: task %s failed: %v", name, err)
			}
			return
		}
		if taskCtx.Err() != nil {
			record.markCanceled(taskCtx.Err())
		} else {
			record.markCompleted()
		}
	}()

	return TaskHandle{
		Name:   name,
		cancel: cancel,
		done:   done,
		mon:    m,
	}
}

// Snapshot returns a point-in-time view of monitored goroutines.
func (m *Monitor) Snapshot() MonitorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := MonitorStatus{
		StartedAt: m.startedAt,
		Tasks:     make([]TaskStatus, 0, len(m.tasks)),
	}

	for _, task := range m.tasks {
		status.Tasks = append(status.Tasks, task.status())
	}
	return status
}

// Stop cancels all tasks and waits for them to exit.
func (m *Monitor) Stop() {
	m.cancel()
	m.wg.Wait()
}

func (m *Monitor) taskStatus(name string) TaskStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if task, ok := m.tasks[name]; ok {
		return task.status()
	}
	return TaskStatus{Name: name}
}

func (m *Monitor) watchdog() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.inspectTasks()
		}
	}
}

func (m *Monitor) inspectTasks() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, task := range m.tasks {
		task.mu.Lock()
		if task.state == TaskStateRunning {
			since := now.Sub(task.lastHeartbeat)
			if m.stallThreshold > 0 && since > m.stallThreshold {
				if !task.heartbeatStalled {
					task.heartbeatStalled = true
					m.logger.Printf("monitor: task %s stalled (%s without heartbeat)", task.name, since.Truncate(time.Millisecond))
				}
			} else if task.heartbeatStalled {
				task.heartbeatStalled = false
				m.logger.Printf("monitor: task %s recovered after stall", task.name)
			}
		}
		task.mu.Unlock()
	}
}

type heartbeat struct {
	task *taskRecord
}

func (h *heartbeat) Tick() {
	h.task.touch()
}

type taskRecord struct {
	name             string
	start            time.Time
	end              time.Time
	lastHeartbeat    time.Time
	state            TaskState
	errMsg           string
	panicMsg         string
	heartbeatStalled bool
	mu               sync.RWMutex
}

func newTaskRecord(name string) *taskRecord {
	now := time.Now()
	return &taskRecord{
		name:          name,
		start:         now,
		lastHeartbeat: now,
		state:         TaskStateRunning,
	}
}

func (t *taskRecord) touch() {
	t.mu.Lock()
	t.lastHeartbeat = time.Now()
	t.mu.Unlock()
}

func (t *taskRecord) markFailed(err error) {
	t.mu.Lock()
	t.state = TaskStateFailed
	t.end = time.Now()
	t.errMsg = err.Error()
	t.mu.Unlock()
}

func (t *taskRecord) markCompleted() {
	t.mu.Lock()
	t.state = TaskStateCompleted
	t.end = time.Now()
	t.mu.Unlock()
}

func (t *taskRecord) markCanceled(err error) {
	t.mu.Lock()
	t.state = TaskStateCanceled
	t.end = time.Now()
	if err != nil {
		t.errMsg = err.Error()
	}
	t.mu.Unlock()
}

func (t *taskRecord) markPanicked(msg string) {
	t.mu.Lock()
	t.state = TaskStatePanicked
	t.end = time.Now()
	t.panicMsg = msg
	t.mu.Unlock()
}

func (t *taskRecord) status() TaskStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return TaskStatus{
		Name:             t.name,
		State:            t.state,
		StartTime:        t.start,
		EndTime:          t.end,
		LastHeartbeat:    t.lastHeartbeat,
		Error:            t.errMsg,
		Panic:            t.panicMsg,
		HeartbeatStalled: t.heartbeatStalled,
	}
}
