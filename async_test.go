package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// stubExtractFn returns a fixed set of facts for testing without calling Ollama.
func stubExtractFn(text string) ([]ExtractedFact, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return []ExtractedFact{
		{Content: text, Summary: "stub fact", Tags: []string{"stub"}},
	}, nil
}

// failExtractFn always fails, for testing error handling.
func failExtractFn(text string) ([]ExtractedFact, error) {
	return nil, fmt.Errorf("intentional extraction failure")
}

func TestNewAsyncExtractorDefaults(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 0)
	ae.extractFn = stubExtractFn
	defer ae.Stop()

	if ae.storage != store {
		t.Fatal("NewAsyncExtractor().storage mismatch")
	}
	if ae.stopped {
		t.Fatal("NewAsyncExtractor().stopped = true, want false")
	}
	if ae.extractFn == nil {
		t.Fatal("NewAsyncExtractor().extractFn is nil")
	}
}

func TestAsyncExtractorSubmitCreatesJob(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	ae.extractFn = stubExtractFn
	defer ae.Stop()

	jobID, err := ae.Submit("test text", false)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if jobID == "" {
		t.Fatal("Submit() returned empty jobID")
	}
	if !strings.HasPrefix(jobID, "extract-") {
		t.Fatalf("Submit() jobID = %q, want prefix extract-", jobID)
	}

	// Wait for the worker to process the job.
	time.Sleep(200 * time.Millisecond)

	st, ok := ae.JobStatus(jobID)
	if !ok {
		t.Fatalf("JobStatus(%q) not found", jobID)
	}
	if st.Status != "done" {
		t.Fatalf("JobStatus().Status = %q, want done", st.Status)
	}
	if st.ID != jobID {
		t.Fatalf("JobStatus().ID = %q, want %q", st.ID, jobID)
	}
	if len(st.Facts) != 1 {
		t.Fatalf("JobStatus().Facts = %d, want 1", len(st.Facts))
	}
}

func TestAsyncExtractorSubmitAndSave(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	ae.extractFn = stubExtractFn
	defer ae.Stop()

	jobID, err := ae.Submit("save this fact", true)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	// Wait for async save.
	time.Sleep(500 * time.Millisecond)

	st, ok := ae.JobStatus(jobID)
	if !ok {
		t.Fatalf("JobStatus(%q) not found", jobID)
	}
	if st.Status != "done" {
		t.Fatalf("JobStatus().Status = %q, want done", st.Status)
	}
	if len(st.Keys) == 0 {
		t.Fatal("JobStatus().Keys is empty, want saved keys")
	}
}

func TestAsyncExtractorJobStatusMissing(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	ae.extractFn = stubExtractFn
	defer ae.Stop()

	_, ok := ae.JobStatus("extract-nonexistent")
	if ok {
		t.Fatal("JobStatus(nonexistent) = true, want false")
	}
}

func TestAsyncExtractorExtractionFailure(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	ae.extractFn = failExtractFn
	defer ae.Stop()

	jobID, err := ae.Submit("this will fail", false)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	st, ok := ae.JobStatus(jobID)
	if !ok {
		t.Fatalf("JobStatus(%q) not found", jobID)
	}
	if st.Status != "failed" {
		t.Fatalf("JobStatus().Status = %q, want failed", st.Status)
	}
	if st.Error == "" {
		t.Fatal("JobStatus().Error is empty, want error message")
	}
}

func TestAsyncExtractorStopDrains(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	ae.extractFn = stubExtractFn

	// Submit a job then stop immediately.
	ae.Submit("quick stop test", false)

	done := make(chan struct{})
	go func() {
		ae.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5s")
	}

	if !ae.stopped {
		t.Fatal("Stop() did not set stopped flag")
	}
}

func TestAsyncExtractorStoppedSubmit(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	ae.extractFn = stubExtractFn
	ae.Stop()

	// After Stop, Submit should fall back to synchronous extraction.
	jobID, err := ae.Submit("fallback test", false)
	if err != nil {
		t.Fatalf("Submit() after Stop error = %v", err)
	}
	if jobID == "" {
		t.Fatal("Submit() after Stop returned empty jobID")
	}

	st, ok := ae.JobStatus(jobID)
	if !ok {
		t.Fatalf("JobStatus(%q) not found after fallback", jobID)
	}
	if st.Status != "done" && st.Status != "failed" {
		t.Fatalf("JobStatus().Status = %q, want done or failed", st.Status)
	}
}

func TestAsyncExtractorConcurrentSubmitStop(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 64)
	ae.extractFn = func(text string) ([]ExtractedFact, error) {
		// Slow enough to create overlap with Stop.
		time.Sleep(10 * time.Millisecond)
		return stubExtractFn(text)
	}

	// Submit many jobs concurrently, then stop concurrently.
	for i := 0; i < 50; i++ {
		go ae.Submit("concurrent", false)
	}

	time.Sleep(50 * time.Millisecond)
	go ae.Stop()

	// Wait for Stop to complete or timeout.
	done := make(chan struct{})
	go func() {
		ae.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5s under concurrent Submit")
	}
}

func TestStorageEnableAsyncExtractor(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	store.EnableAsyncExtractor(8)
	if store.asyncExtractor == nil {
		t.Fatal("EnableAsyncExtractor() did not set asyncExtractor")
	}

	// Second call should be a no-op and not panic.
	store.EnableAsyncExtractor(16)
}

func TestStorageSubmitExtractDisabled(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	jobID, err := store.SubmitExtract("disabled", false)
	if err != nil {
		t.Fatalf("SubmitExtract(disabled) error = %v", err)
	}
	if jobID != "sync-direct" {
		t.Fatalf("SubmitExtract(disabled) jobID = %q, want sync-direct", jobID)
	}
}

func TestStorageExtractJobStatusDisabled(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	_, err := store.ExtractJobStatus("extract-1")
	if err == nil {
		t.Fatal("ExtractJobStatus(disabled) error = nil, want error")
	}
}
