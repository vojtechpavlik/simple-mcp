// Copyright (c) 2025 Vojtech Pavlik <vojtech@suse.com>
//
// Created using AI tools
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package main provides state management for long-running asynchronous tasks.
// It allows the MCP server to track the status of operations like system upgrades
// or reboots, which exceed the typical timeout for synchronous tool calls.
package task

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// AsyncTask represents the state of a single background job.
type AsyncTask struct {
	ID        string
	ToolName  string
	Status    string // "pending", "running", "completed", "failed"
	Message   string // Final output or error message
	StartTime time.Time
	EndTime   time.Time
}

// TaskStore is a thread-safe registry for managing async tasks.
type TaskStore struct {
	mu       sync.RWMutex
	tasks    map[string]*AsyncTask
	maxTasks int
}

func NewTaskStore(maxTasks int) *TaskStore {
	return &TaskStore{
		tasks:    make(map[string]*AsyncTask),
		maxTasks: maxTasks,
	}
}

// PrepareSlot ensures there is room for a new task in the store.
// If the store is full, it tries to find the oldest finished task to evict.
// It returns the ID of the evicted task if successful, or an empty string if no eviction was needed.
// It returns an error if the store is full and no task can be evicted (all tasks are active).
func (ts *TaskStore) PrepareSlot() (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if len(ts.tasks) < ts.maxTasks {
		return "", nil
	}

	var oldestTask *AsyncTask
	for _, task := range ts.tasks {
		if task.Status == "completed" || task.Status == "failed" {
			if oldestTask == nil || task.EndTime.Before(oldestTask.EndTime) {
				oldestTask = task
			}
		}
	}

	if oldestTask == nil {
		return "", fmt.Errorf("Too many asynchronous tasks. Maximum allowed: %d", ts.maxTasks)
	}

	return oldestTask.ID, nil
}

// Create initializes a new task in the "pending" state.
func (ts *TaskStore) Create(id string, toolName string) *AsyncTask {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	task := &AsyncTask{
		ID:        id,
		ToolName:  toolName,
		Status:    "pending",
		Message:   "Job has been queued.",
		StartTime: time.Now(),
	}
	ts.tasks[strings.ToLower(id)] = task
	return task
}

// Delete removes a task from the store.
func (ts *TaskStore) Delete(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tasks, id)
}

func (ts *TaskStore) Get(id string) (*AsyncTask, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	task, ok := ts.tasks[strings.ToLower(id)]
	return task, ok
}

// SetStatus updates the state and output message of a task.
func (ts *TaskStore) SetStatus(id string, status string, message string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	task, ok := ts.tasks[strings.ToLower(id)]
	if !ok {
		return
	}

	task.Status = status
	task.Message = message
	if status == "completed" || status == "failed" {
		task.EndTime = time.Now()
	}
}

// ListActiveTasks returns a slice of all currently pending or running tasks.
// This powers the 'ListPendingTasks' tool, helping the LLM recover lost task IDs.
func (ts *TaskStore) ListActiveTasks() []*AsyncTask {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var activeTasks []*AsyncTask
	for _, task := range ts.tasks {
		if task.Status == "pending" || task.Status == "running" {
			activeTasks = append(activeTasks, task)
		}
	}
	return activeTasks
}

// HasActiveTask checks if a specific tool type is already running.
// Used to implement a concurrency lock (e.g., preventing parallel upgrades).
func (ts *TaskStore) HasActiveTask(toolName string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	for _, task := range ts.tasks {
		if task.ToolName == toolName && (task.Status == "pending" || task.Status == "running") {
			return true
		}
	}
	return false
}

// FormatStatus returns a human-readable summary of the task.
func (t *AsyncTask) FormatStatus() string {
	var duration time.Duration
	if t.Status == "completed" || t.Status == "failed" {
		if !t.EndTime.IsZero() {
			duration = t.EndTime.Sub(t.StartTime)
		}
	} else {
		duration = time.Since(t.StartTime)
	}
	durationStr := duration.Truncate(time.Second).String()

	switch t.Status {
	case "completed":
		return fmt.Sprintf("Status: %s\nCompleted In: %s\nOutput: %s", t.Status, durationStr, t.Message)
	case "failed":
		return fmt.Sprintf("Status: %s\nFailed After: %s\nError: %s", t.Status, durationStr, t.Message)
	default:
		return fmt.Sprintf("Status: %s\nRunning For: %s\nMessage: %s", t.Status, durationStr, t.Message)
	}
}
