// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
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
