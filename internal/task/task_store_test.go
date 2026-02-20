package task

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTaskStore_CreateAndGet(t *testing.T) {
	ts := NewTaskStore(10)
	id := "job-123"
	tool := "SystemUpgrade"

	task := ts.Create(id, tool)
	if task.ID != id {
		t.Errorf("expected ID %s, got %s", id, task.ID)
	}
	if task.Status != "pending" {
		t.Errorf("expected status pending, got %s", task.Status)
	}

	retrieved, ok := ts.Get(id)
	if !ok {
		t.Fatalf("failed to retrieve task %s", id)
	}
	if retrieved != task {
		t.Errorf("retrieved task pointer mismatch")
	}

	// Test case-insensitivity
	retrievedLower, ok := ts.Get(strings.ToLower(id))
	if !ok {
		t.Errorf("failed to retrieve task with lowercase ID")
	}
	if retrievedLower != task {
		t.Errorf("retrieved lowercase task pointer mismatch")
	}

	retrievedUpper, ok := ts.Get(strings.ToUpper(id))
	if !ok {
		t.Errorf("failed to retrieve task with uppercase ID")
	}
	if retrievedUpper != task {
		t.Errorf("retrieved uppercase task pointer mismatch")
	}
}

func TestTaskStore_Concurrency(t *testing.T) {
	// this test verifies the store doesn't panic under concurrent access
	ts := NewTaskStore(100)
	var wg sync.WaitGroup

	// concurrently create tasks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("job-%d", i)
			ts.Create(id, "test-tool")
			ts.SetStatus(id, "running", "working")
			_, _ = ts.Get(id)
		}(i)
	}

	wg.Wait()
	
	tasks := ts.ListActiveTasks()
	// All tasks are pending/running, so we expect 100 active tasks
	if len(tasks) != 100 {
		t.Errorf("expected 100 active tasks, got %d", len(tasks))
	}
}

func TestTaskStore_HasActiveTask(t *testing.T) {
	ts := NewTaskStore(10)
	ts.Create("1", "Upgrade")
	ts.SetStatus("1", "running", "...")

	if !ts.HasActiveTask("Upgrade") {
		t.Error("expected HasActiveTask to return true")
	}

	ts.SetStatus("1", "completed", "done")
	if ts.HasActiveTask("Upgrade") {
		t.Error("expected HasActiveTask to return false after completion")
	}
}

func TestTaskStore_Vacuum(t *testing.T) {
	ts := NewTaskStore(2)

	// 1. Fill the store with active tasks
	ts.Create("1", "Tool1")
	ts.SetStatus("1", "running", "...")
	ts.Create("2", "Tool2")
	ts.SetStatus("2", "running", "...")

	// 2. Try to prepare slot, should fail
	_, err := ts.PrepareSlot()
	if err == nil {
		t.Error("expected error when store is full of active tasks")
	}

	// 3. Complete one task
	ts.SetStatus("1", "completed", "done")
	// Ensure EndTime is set and different
	task1, _ := ts.Get("1")
	task1.EndTime = task1.EndTime.Add(-10 * time.Second)

	// 4. Complete another task later
	ts.SetStatus("2", "failed", "error")
	task2, _ := ts.Get("2")
	task2.EndTime = task2.EndTime.Add(-5 * time.Second)

	// 5. Prepare slot, should return task 1 (oldest)
	evictedID, err := ts.PrepareSlot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evictedID != "1" {
		t.Errorf("expected evicted task 1, got %s", evictedID)
	}

	// 6. Delete task 1 and create a new one to fill the store again
	ts.Delete("1")
	ts.Create("3", "Tool3")
	ts.SetStatus("3", "running", "...")

	// 7. Prepare slot again, should return task 2 (the only completed/failed one)
	evictedID, err = ts.PrepareSlot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evictedID != "2" {
		t.Errorf("expected evicted task 2, got %s", evictedID)
	}
}
