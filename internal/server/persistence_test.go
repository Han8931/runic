package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/han/runic/internal/protocol"
)

// save writes a snapshot only when the queue changed since the last save.
func TestSaveSkipsWhenClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.json")
	q := NewJobQueue()

	if err := q.save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("a never-modified queue should not write a snapshot")
	}

	q.Add(protocol.NewJobRequest{Command: "a"})
	if err := q.save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dirty queue should write a snapshot: %v", err)
	}

	// No mutations since the last save: saving again must be a no-op.
	os.Remove(path)
	if err := q.save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("save should skip while the queue is clean")
	}
}

func TestJobQueueSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	q := NewJobQueue()
	q.CreateGroup("work")
	q.CreateSession("build")
	q.MoveSession("build", "work")
	first := q.Add(protocol.NewJobRequest{
		Command:     "make build",
		CommandArgs: []string{"make", "build"},
		Session:     "build",
		Label:       "compile",
		NumSlots:    2,
	})
	q.MarkFinished(first.ID, protocol.Result{ExitCode: 0})
	q.Add(protocol.NewJobRequest{Command: "make test", DependOn: []int{first.ID}})

	if err := q.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	restored, err := loadJobQueue(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	jobs := restored.AllInfo()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Label != "compile" || jobs[0].Session != "build" || jobs[0].NumSlots != 2 {
		t.Fatalf("job metadata was not restored: %+v", jobs[0])
	}
	if jobs[1].State != protocol.StateQueued || len(jobs[1].DependOn) != 1 {
		t.Fatalf("queued job was not restored: %+v", jobs[1])
	}
	if got := restored.AllSessionInfo(); len(got) != 2 {
		t.Fatalf("expected sessions to be restored, got %+v", got)
	}
	if next := restored.Add(protocol.NewJobRequest{Command: "next"}); next.ID != 2 {
		t.Fatalf("expected next ID 2, got %d", next.ID)
	}
}

func TestLoadJobQueueMarksRunningJobsInterrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	q := NewJobQueue()
	job := q.Add(protocol.NewJobRequest{Command: "long-running"})
	q.SetRunning(job.ID, 1234, "output.log")
	if err := q.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored, err := loadJobQueue(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	info, ok := restored.GetInfo(job.ID)
	if !ok {
		t.Fatal("restored job not found")
	}
	if info.State != protocol.StateFinished || info.Result.ExitCode != -1 || info.PID != 0 {
		t.Fatalf("running job was not marked interrupted: %+v", info)
	}
}

func TestLoadJobQueueRejectsCorruptSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadJobQueue(path); err == nil {
		t.Fatal("expected corrupt snapshot to be rejected")
	}
}

func TestLoadJobQueueMissingSnapshotUsesEmptyQueue(t *testing.T) {
	q, err := loadJobQueue(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if jobs := q.AllInfo(); len(jobs) != 0 {
		t.Fatalf("expected empty queue, got %+v", jobs)
	}
}

// Snapshot saves must be strictly serialized. Encoding under the queue lock and
// writing outside it lets two concurrent saves write out of order, so an older
// snapshot can land on top of a newer one and silently lose jobs. The race
// detector cannot see that — the overlap is in file order — so this test drives
// it directly: it holds each write open long enough for another to start, and
// fails if one ever does, or if the bytes reaching disk go backwards.
func TestSaveSerializesConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.json")
	q := NewJobQueue()

	var mu sync.Mutex
	inWrite := 0
	var overlapped bool
	var seen []int // NextID of each snapshot, in the order it was written

	defer func(orig func(string, []byte) error) { snapshotWriter = orig }(snapshotWriter)
	snapshotWriter = func(p string, data []byte) error {
		var snap queueSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			t.Errorf("unmarshal snapshot: %v", err)
		}
		mu.Lock()
		inWrite++
		if inWrite > 1 {
			overlapped = true
		}
		seen = append(seen, snap.NextID)
		mu.Unlock()

		time.Sleep(5 * time.Millisecond) // a window for another save to barge in
		mu.Lock()
		inWrite--
		mu.Unlock()
		return writeSnapshot(p, data)
	}

	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Add(protocol.NewJobRequest{Command: "job"})
			if err := q.save(path); err != nil {
				t.Errorf("save: %v", err)
			}
		}()
	}
	wg.Wait()

	if overlapped {
		t.Error("two saves wrote the snapshot at the same time")
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("snapshot %d went backwards (next_id %d after %d): an older "+
				"snapshot overwrote a newer one", i, seen[i], seen[i-1])
		}
	}

	// Whatever the interleaving, the file on disk must describe every job.
	loaded, err := loadJobQueue(path)
	if err != nil {
		t.Fatalf("loadJobQueue: %v", err)
	}
	if got := len(loaded.AllInfo()); got != n {
		t.Errorf("snapshot holds %d jobs, want %d — a save was lost", got, n)
	}
}
