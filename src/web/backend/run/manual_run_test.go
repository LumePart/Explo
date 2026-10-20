package run

import (
	"context"
	"os"
	"testing"
	"time"

	"explo/src/web/backend/app"
)

// Zero means a run is never killed on a timer, only by the stop endpoint.
func TestRunContext_ZeroLeavesTheRunUnbounded(t *testing.T) {
	ctx, cancel := runContext(0)
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Error("a zero timeout produced a deadline; runs would be killed unexpectedly")
	}

	select {
	case <-ctx.Done():
		t.Error("context was already done")
	default:
	}
}

// A configured timeout has to actually bound the run.
func TestRunContext_AppliesTheTimeout(t *testing.T) {
	ctx, cancel := runContext(30 * time.Millisecond)
	defer cancel()

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("no deadline was set, so a wedged run would never be killed")
	}

	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("ctx.Err() = %v, want DeadlineExceeded", ctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Error("the deadline never fired")
	}
}

// The guard that refuses a second run must clear itself when a wedged run is
// killed, or the timeout buys nothing: every later run is still refused.
func TestStartRun_TimeoutReleasesTheAlreadyRunningGuard(t *testing.T) {
	mr := NewManualRun(app.Config{
		// A command that would otherwise run far longer than the test.
		ExploPath:  "/bin/sleep",
		RunTimeout: 50 * time.Millisecond,
	})

	if err := mr.startRun([]string{"30"}); err != nil {
		t.Fatalf("startRun() = %v, want nil", err)
	}
	if !mr.currentRunStatus().Running {
		t.Fatal("run did not register as running")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !mr.currentRunStatus().Running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Error("the run was still marked running long after its timeout; later runs would keep being refused")
}

// Back-to-back runs must be possible once the first has been cut short.
func TestStartRun_SecondRunIsAcceptedAfterATimeout(t *testing.T) {
	mr := NewManualRun(app.Config{
		ExploPath:  "/bin/sleep",
		RunTimeout: 50 * time.Millisecond,
	})

	if err := mr.startRun([]string{"30"}); err != nil {
		t.Fatalf("first startRun() = %v, want nil", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && mr.currentRunStatus().Running {
		time.Sleep(5 * time.Millisecond)
	}

	if err := mr.startRun([]string{"0"}); err != nil {
		t.Errorf("second startRun() = %v, want nil once the first was killed", err)
	}
}

// BUG-21: the already-running branch closed pw a second time instead of
// the log file, leaking one descriptor per rejected attempt.
func TestStartRun_RejectedRunsDoNotLeakFileHandles(t *testing.T) {
	mr := NewManualRun(app.Config{
		ExploPath:  "/bin/sleep",
		RunTimeout: 5 * time.Second,
		WebDataDir: t.TempDir(),
	})

	if err := mr.startRun([]string{"30"}); err != nil {
		t.Fatalf("startRun() = %v, want nil", err)
	}
	t.Cleanup(func() {
		mr.state.mu.Lock()
		cancel := mr.state.cancel
		mr.state.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})

	countFDs := func() int {
		entries, err := os.ReadDir("/dev/fd")
		if err != nil {
			t.Skipf("/dev/fd unavailable: %v", err)
		}
		return len(entries)
	}

	before := countFDs()
	for i := 0; i < 5; i++ {
		if err := mr.startRun([]string{"30"}); err != errRunAlreadyStarted {
			t.Fatalf("rejected startRun() = %v, want errRunAlreadyStarted", err)
		}
	}
	if after := countFDs(); after > before {
		t.Errorf("open FDs grew %d -> %d over 5 rejected runs; each rejection leaks a handle", before, after)
	}
}

// While a run really is in flight, a second must still be refused.
func TestStartRun_RefusesAConcurrentRun(t *testing.T) {
	mr := NewManualRun(app.Config{
		ExploPath:  "/bin/sleep",
		RunTimeout: 5 * time.Second,
	})

	if err := mr.startRun([]string{"30"}); err != nil {
		t.Fatalf("startRun() = %v, want nil", err)
	}
	t.Cleanup(func() {
		mr.state.mu.Lock()
		cancel := mr.state.cancel
		mr.state.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})

	if err := mr.startRun([]string{"30"}); err != errRunAlreadyStarted {
		t.Errorf("second startRun() = %v, want errRunAlreadyStarted", err)
	}
}
