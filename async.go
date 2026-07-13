// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// AsyncWriter — non-blocking writes for memory operations.
//
// Purpose: queue memory Save operations so the MCP tool handler returns
// immediately instead of waiting for embedding generation + SQLite write.
// Critical for voice mode (Alice) where every millisecond of latency matters.
//
// Key generation happens synchronously (cheap: md5 + date).
// Only the SetWithEmbedding call is deferred to a background worker.
// ---------------------------------------------------------------------------

// AsyncWriteRequest represents a queued write operation.
type AsyncWriteRequest struct {
	Key   string
	Value *MemoryValue
	Text  string
}

// AsyncWriter manages a pool of background workers that perform memory writes.
type AsyncWriter struct {
	queue   chan *AsyncWriteRequest
	storage *Storage
	wg      sync.WaitGroup
	stopped bool
}

// NewAsyncWriter creates an AsyncWriter with the given queue depth and worker
// count. The caller must call Stop() to drain and shut down workers.
func NewAsyncWriter(s *Storage, queueDepth, workers int) *AsyncWriter {
	if queueDepth <= 0 {
		queueDepth = 64
	}
	if workers <= 0 {
		workers = 1
	}

	aw := &AsyncWriter{
		queue:   make(chan *AsyncWriteRequest, queueDepth),
		storage: s,
	}

	for i := 0; i < workers; i++ {
		aw.wg.Add(1)
		go aw.worker(i)
	}

	log.Printf("✅ AsyncWriter started: %d workers, queue depth %d", workers, queueDepth)
	return aw
}

// worker consumes write requests from the queue and executes them.
func (aw *AsyncWriter) worker(id int) {
	defer aw.wg.Done()
	for req := range aw.queue {
		start := time.Now()
		_, err := aw.storage.Save(req.Key, req.Value, req.Text, false)
		elapsed := time.Since(start)
		if err != nil {
			log.Printf("⚠ [async-writer/%d] Save failed for key %q: %v (elapsed: %v)",
				id, req.Key, err, elapsed)
		} else {
			log.Printf("  [async-writer/%d] Saved %q (elapsed: %v)", id, req.Key, elapsed)
		}
	}
}

// Save queues a memory write operation. Key generation is performed
// synchronously (cheap); the actual storage write happens in a background
// goroutine. Returns the generated key immediately.
//
// If the queue is full, falls back to a synchronous write to avoid blocking
// the caller indefinitely.
func (aw *AsyncWriter) Save(key string, value *MemoryValue, text string, autoGenKey bool) (string, error) {
	if aw.stopped {
		// Fall back to synchronous write if the writer is stopped.
		return aw.storage.Save(key, value, text, autoGenKey)
	}

	// Generate the final key synchronously (cheap operation).
	if value.Timestamp == "" {
		value.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	finalKey := key
	if autoGenKey || key == "" {
		finalKey = autoKey(text)
	}

	// Copy the value so the caller can reuse the pointer without races.
	valCopy := *value

	req := &AsyncWriteRequest{
		Key:   finalKey,
		Value: &valCopy,
		Text:  text,
	}

	select {
	case aw.queue <- req:
		return finalKey, nil
	default:
		// Queue full — fall back to synchronous write.
		log.Printf("⚠ [async-writer] queue full, falling back to sync write for %q", finalKey)
		return aw.storage.Save(finalKey, &valCopy, text, false)
	}
}

// Stop gracefully shuts down the writer, waiting for all queued writes to
// complete. The underlying storage is NOT closed — callers must close it
// separately.
func (aw *AsyncWriter) Stop() {
	if aw.stopped {
		return
	}
	aw.stopped = true
	close(aw.queue)
	aw.wg.Wait()
	log.Printf("✅ AsyncWriter stopped, all queued writes completed")
}

// ───────────────────────────────────────────────────────────────────────────
// AsyncExtractor — background LLM fact extraction workers.
//
// Purpose: queue memory_extract calls so the MCP handler returns immediately
// instead of waiting for the LLM to extract facts. The LLM call can take 10-120
// seconds and the MCP gateway kills the HTTP connection before it finishes.
//
// Max 1 concurrent worker — qwen2.5-coder:7b uses all 24 CPU cores at 100%.
// Job lifecycle: pending → running → done | failed.
// ───────────────────────────────────────────────────────────────────────────

// ExtractRequest represents a queued extraction job.
type ExtractRequest struct {
	Text     string
	AutoSave bool
	JobID    string
}

// ExtractJobStatus is the current state of an extraction job.
type ExtractJobStatus struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"` // pending, running, done, failed
	Keys      []string  `json:"keys,omitempty"`
	Facts     []ExtractedFact `json:"facts,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AsyncExtractor manages a single background worker that performs fact
// extraction in the background.
type AsyncExtractor struct {
	queue     chan *ExtractRequest
	storage   *Storage
	wg        sync.WaitGroup
	stopped   bool
	stopMu    sync.RWMutex // guards stopped flag and channel close/send coordination
	mu        sync.Mutex    // guards jobs map
	jobs      map[string]*ExtractJobStatus
	extractFn func(string) ([]ExtractedFact, error)
}

// NewAsyncExtractor creates an AsyncExtractor with 1 worker and the given
// queue depth. Must call Stop() to drain jobs and shut down.
func NewAsyncExtractor(s *Storage, queueDepth int) *AsyncExtractor {
	if queueDepth <= 0 {
		queueDepth = 64
	}
	ae := &AsyncExtractor{
		queue:     make(chan *ExtractRequest, queueDepth),
		storage:   s,
		jobs:      make(map[string]*ExtractJobStatus),
		extractFn: ExtractFactsAsync,
	}
	ae.wg.Add(1)
	go ae.worker()
	log.Printf("✅ AsyncExtractor started: 1 worker, queue depth %d", queueDepth)
	return ae
}

// Submit queues an extraction job and returns its ID immediately.
// If the extractor is stopped or the queue is full, falls back to synchronous
// extraction so no data is lost.
func (ae *AsyncExtractor) Submit(text string, autoSave bool) (string, error) {
	jobID := fmt.Sprintf("extract-%d", time.Now().UnixNano())

	ae.mu.Lock()
	ae.jobs[jobID] = &ExtractJobStatus{
		ID:        jobID,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	ae.mu.Unlock()

	// Hold the read lock while checking stopped and sending to the queue.
	// Stop() uses a write lock, so it cannot close the channel between the
	// check and the send; this prevents a send-on-closed-channel panic.
	ae.stopMu.RLock()
	if ae.stopped {
		ae.stopMu.RUnlock()
		return ae.fallbackSyncExtract(jobID, text, autoSave)
	}

	req := &ExtractRequest{Text: text, AutoSave: autoSave, JobID: jobID}

	select {
	case ae.queue <- req:
		ae.stopMu.RUnlock()
		return jobID, nil
	default:
		ae.stopMu.RUnlock()
		// Queue full — fall back to sync extraction so we don't drop data.
		log.Printf("⚠ [async-extractor] queue full, falling back to sync extraction")
		return ae.fallbackSyncExtract(jobID, text, autoSave)
	}
}

// fallbackSyncExtract performs synchronous extraction when the async queue
// cannot accept the job. Updates the tracked job status and returns the job ID.
// Mirrors the worker's extraction/save split so the autoSave flag is respected.
func (ae *AsyncExtractor) fallbackSyncExtract(jobID, text string, autoSave bool) (string, error) {
	facts, err := ae.extractFn(text)
	if err != nil {
		ae.updateJob(jobID, "failed", nil, nil, err.Error())
		return jobID, err
	}

	var keys []string
	if autoSave {
		keys = ae.storage.saveExtractedFacts(facts)
	}

	ae.updateJob(jobID, "done", keys, facts, "")
	return jobID, nil
}

// JobStatus returns a copy of the current status of a job, if it exists.
func (ae *AsyncExtractor) JobStatus(jobID string) (ExtractJobStatus, bool) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	st, ok := ae.jobs[jobID]
	if !ok {
		return ExtractJobStatus{}, false
	}
	// Return a copy so callers cannot race with the worker updating it.
	return *st, true
}

// updateJob updates a tracked job status in a thread-safe way.
func (ae *AsyncExtractor) updateJob(jobID, status string, keys []string, facts []ExtractedFact, errMsg string) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	if st, ok := ae.jobs[jobID]; ok {
		st.Status = status
		st.Keys = keys
		st.Facts = facts
		st.Error = errMsg
		st.UpdatedAt = time.Now().UTC()
	}
}

// worker processes extraction jobs from the queue sequentially.
func (ae *AsyncExtractor) worker() {
	defer ae.wg.Done()
	for req := range ae.queue {
		ae.updateJob(req.JobID, "running", nil, nil, "")
		start := time.Now()

		facts, err := ae.extractFn(req.Text)
		elapsed := time.Since(start)

		if err != nil {
			log.Printf("⚠ [async-extractor] extraction failed (job %s, elapsed %v): %v",
				req.JobID, elapsed, err)
			ae.updateJob(req.JobID, "failed", nil, nil, err.Error())
			continue
		}

		var keys []string
		if req.AutoSave {
			keys = ae.storage.saveExtractedFacts(facts)
		}

		log.Printf("  [async-extractor] extracted %d facts (job %s, elapsed %v, saved %d)",
			len(facts), req.JobID, elapsed, len(keys))
		ae.updateJob(req.JobID, "done", keys, facts, "")
	}
}

// Stop gracefully shuts down the extractor. The worker finishes the current
// job and drains any pending jobs from the queue before returning.
func (ae *AsyncExtractor) Stop() {
	ae.stopMu.Lock()
	defer ae.stopMu.Unlock()
	if ae.stopped {
		return
	}
	ae.stopped = true
	close(ae.queue)
	ae.wg.Wait()
	log.Printf("✅ AsyncExtractor stopped")
}
