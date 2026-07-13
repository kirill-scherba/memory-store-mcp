package main

import (
	"strings"
	"testing"
	"time"
)

func TestNewAsyncExtractorDefaults(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 0)
	defer ae.Stop()

	if ae.storage != store {
		t.Fatal("NewAsyncExtractor().storage mismatch")
	}
	if ae.stopped {
		t.Fatal("NewAsyncExtractor().stopped = true, want false")
	}
}

func TestAsyncExtractorSubmitCreatesJob(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
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

	st, ok := ae.JobStatus(jobID)
	if !ok {
		t.Fatalf("JobStatus(%q) not found", jobID)
	}
	if st.Status != "pending" && st.Status != "running" && st.Status != "done" {
		t.Fatalf("JobStatus().Status = %q, want pending/running/done", st.Status)
	}
	if st.ID != jobID {
		t.Fatalf("JobStatus().ID = %q, want %q", st.ID, jobID)
	}
}

func TestAsyncExtractorJobStatusMissing(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)
	defer ae.Stop()

	_, ok := ae.JobStatus("extract-nonexistent")
	if ok {
		t.Fatal("JobStatus(nonexistent) = true, want false")
	}
}

func TestAsyncExtractorStopDrains(t *testing.T) {
	store, _ := NewStorage(t.TempDir() + "/memory.db")
	defer store.Close()

	ae := NewAsyncExtractor(store, 16)

	// Submit a job then stop immediately. With Ollama unavailable the worker may
	// fail quickly; the important part is that Stop returns without hanging.
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
	ae.Stop()

	// After Stop, Submit should fall back to synchronous extraction.
	jobID, err := ae.Submit("fallback test", false)
	if err != nil {
		t.Fatalf("Submit() after Stop error = %v", err)
	}
	if jobID == "" {
		t.Fatal("Submit() after Stop returned empty jobID")
	}

	// Give fallback a moment to update status.
	time.Sleep(50 * time.Millisecond)
	st, ok := ae.JobStatus(jobID)
	if !ok {
		t.Fatalf("JobStatus(%q) not found after fallback", jobID)
	}
	// With autoSave=false and empty text there are no facts, so it should be done.
	if st.Status != "done" && st.Status != "failed" {
		t.Fatalf("JobStatus().Status = %q, want done or failed", st.Status)
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
