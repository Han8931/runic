package server

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/han/runic/internal/protocol"
)

// A job exceeding its --timeout is killed and reported as TimedOut.
func TestExecutorTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell commands")
	}
	q := NewJobQueue()
	e := NewExecutor(t.TempDir())
	j := q.Add(protocol.NewJobRequest{
		Command:     "sleep 5",
		CommandArgs: []string{"sleep", "5"},
		TimeoutMS:   200,
	})
	q.SetRunning(j.ID, 0, "")

	done := make(chan protocol.Result, 1)
	start := time.Now()
	e.Run(ExecRequest{Job: j, JobQueue: q, OnFinish: func(_ int, r protocol.Result) { done <- r }})

	r := <-done
	if !r.TimedOut {
		t.Errorf("expected TimedOut result, got %+v", r)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("timeout should have killed the job promptly, took %v", elapsed)
	}
}

// A failing job with retries is re-run; waiters resolve only on the final
// attempt, and the attempt counter records the retries used.
func TestSchedulerRetries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell commands")
	}
	q := NewJobQueue()
	e := NewExecutor(t.TempDir())
	s := NewScheduler(q, e, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	j := q.Add(protocol.NewJobRequest{
		Command:     "sh -c 'exit 1'",
		CommandArgs: []string{"sh", "-c", "exit 1"},
		Retries:     2,
	})
	s.Poke()

	select {
	case r := <-q.WaitFor(j.ID):
		if r.ExitCode != 1 {
			t.Errorf("expected final exit code 1, got %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("job never finished")
	}
	info, _ := q.GetInfo(j.ID)
	if info.Attempt != 2 {
		t.Errorf("expected 2 retries used, got %d", info.Attempt)
	}
	if info.State != protocol.StateFinished {
		t.Errorf("expected finished after final attempt, got %v", info.State)
	}
}

// A multi-slot job blocked only on free slots blocks younger jobs, so a
// stream of small jobs can't starve it.
func TestJobQueueNextRunnableHeadOfLine(t *testing.T) {
	q := NewJobQueue()
	q.Add(protocol.NewJobRequest{Command: "big", NumSlots: 2})
	q.Add(protocol.NewJobRequest{Command: "small", NumSlots: 1})

	if _, ok := q.NextRunnable(2, 1); ok {
		t.Error("small job must not overtake the slot-blocked big job")
	}
	j, ok := q.NextRunnable(2, 0)
	if !ok || j.Info.Command != "big" {
		t.Error("expected the big job to run once slots free up")
	}
}

// Killing a running job with retries left must finish it, not restart it: the
// signal is indistinguishable from a crash at the exit-status level, so the
// kill has to be recorded for the scheduler to tell them apart.
func TestKilledJobDoesNotRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell commands")
	}
	q := NewJobQueue()
	e := NewExecutor(t.TempDir())
	s := NewScheduler(q, e, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	j := q.Add(protocol.NewJobRequest{
		Command:     "sleep 30",
		CommandArgs: []string{"sleep", "30"},
		Retries:     3,
	})
	s.Poke()

	// Wait for it to actually be running before killing it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if info, ok := q.GetInfo(j.ID); ok && info.State == protocol.StateRunning && info.PID != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never started")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The kill path as the daemon runs it: record the cancellation, then signal.
	if !q.MarkCanceled(j.ID) {
		t.Fatal("MarkCanceled on a running job")
	}
	if err := e.Kill(j.ID); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	select {
	case r := <-q.WaitFor(j.ID):
		if !r.Canceled {
			t.Errorf("result should report the cancellation, got %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("killed job never resolved — it was probably requeued for a retry")
	}

	info, _ := q.GetInfo(j.ID)
	if info.State != protocol.StateFinished {
		t.Errorf("state = %v, want finished (a kill must not requeue)", info.State)
	}
	if info.Attempt != 0 {
		t.Errorf("attempt = %d, want 0: a kill must not spend a retry", info.Attempt)
	}
}

// A kill that finds nothing to signal leaves the job's retries intact — the
// handler takes the mark back.
func TestClearCanceledRestoresRetries(t *testing.T) {
	q := NewJobQueue()
	j := q.Add(protocol.NewJobRequest{Command: "x", Retries: 1})
	q.SetRunning(j.ID, 1234, "")

	q.MarkCanceled(j.ID)
	if q.RequeueForRetry(j.ID) {
		t.Fatal("a canceled job must not requeue")
	}
	q.ClearCanceled(j.ID)
	if !q.RequeueForRetry(j.ID) {
		t.Fatal("clearing the cancellation should restore the retry")
	}
}
