// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kirill-scherba/s3lite"
)

// lockKeyPrefix is the S3-style key namespace for lease-based locks.
const lockKeyPrefix = "memory/locks/"

// LockRecord is the persisted state of a lease-based lock.
type LockRecord struct {
	Owner      string `json:"owner"`
	ExpiresAt  string `json:"expires_at"`
	AcquiredAt string `json:"acquired_at"`
}

// lockKey returns the storage key for a given resource.
func lockKey(resource string) string {
	return lockKeyPrefix + strings.TrimPrefix(resource, lockKeyPrefix)
}

// lockExpired reports whether a lock record is past its expiry.
func lockExpired(rec *LockRecord) bool {
	if rec == nil || rec.ExpiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, rec.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().UTC().After(t)
}

// lockRecord reads the current lock record for a resource, returning nil when
// the lock does not exist or has expired.
func (s *Storage) lockRecord(resource string) (*LockRecord, error) {
	data, err := s.kv.Get(lockKey(resource))
	if err != nil {
		if errors.Is(err, s3lite.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read lock: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var rec LockRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("decode lock: %w", err)
	}
	if lockExpired(&rec) {
		return nil, nil
	}
	return &rec, nil
}

// storeLock persists a lock record under the resource key.
func (s *Storage) storeLock(resource string, rec *LockRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}
	if _, err := s.kv.Set(lockKey(resource), data); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}
	return nil
}

// LockAcquire attempts to acquire a lease-based lock for resource on behalf of
// owner. If the lock is free, expired, or already held by the same owner, it
// succeeds and resets the TTL. A different owner must wait until the lease
// expires or the lock is released.
func (s *Storage) LockAcquire(resource, owner string, ttl time.Duration) (bool, *LockRecord, error) {
	if resource == "" {
		return false, nil, fmt.Errorf("resource is required")
	}
	if owner == "" {
		return false, nil, fmt.Errorf("owner is required")
	}
	if ttl <= 0 {
		return false, nil, fmt.Errorf("ttl must be positive")
	}

	s.lockMu.Lock()
	defer s.lockMu.Unlock()

	rec, err := s.lockRecord(resource)
	if err != nil {
		return false, nil, err
	}

	now := time.Now().UTC()
	if rec != nil && rec.Owner != owner {
		return false, rec, nil
	}

	newRec := &LockRecord{
		Owner:      owner,
		AcquiredAt: now.Format(time.RFC3339),
		ExpiresAt:  now.Add(ttl).Format(time.RFC3339),
	}
	if err := s.storeLock(resource, newRec); err != nil {
		return false, nil, err
	}
	return true, newRec, nil
}

// LockRelease removes a lock, but only if it is currently held by owner.
// Expired locks are ignored.
func (s *Storage) LockRelease(resource, owner string) (bool, error) {
	if resource == "" || owner == "" {
		return false, fmt.Errorf("resource and owner are required")
	}

	s.lockMu.Lock()
	defer s.lockMu.Unlock()

	rec, err := s.lockRecord(resource)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return false, nil
	}
	if rec.Owner != owner {
		return false, nil
	}
	if err := s.kv.Del(lockKey(resource)); err != nil {
		return false, fmt.Errorf("delete lock: %w", err)
	}
	return true, nil
}

// LockExtend renews the TTL of an existing lock, but only if it is held by owner
// and has not already expired.
func (s *Storage) LockExtend(resource, owner string, ttl time.Duration) (bool, *LockRecord, error) {
	if resource == "" || owner == "" {
		return false, nil, fmt.Errorf("resource and owner are required")
	}
	if ttl <= 0 {
		return false, nil, fmt.Errorf("ttl must be positive")
	}

	s.lockMu.Lock()
	defer s.lockMu.Unlock()

	rec, err := s.lockRecord(resource)
	if err != nil {
		return false, nil, err
	}
	if rec == nil {
		return false, nil, nil
	}
	if rec.Owner != owner {
		return false, rec, nil
	}

	rec.ExpiresAt = time.Now().UTC().Add(ttl).Format(time.RFC3339)
	if err := s.storeLock(resource, rec); err != nil {
		return false, nil, err
	}
	return true, rec, nil
}

// LockStatus returns the current, non-expired lock record for a resource, or
// nil when the lock is free or expired.
func (s *Storage) LockStatus(resource string) (*LockRecord, error) {
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}
	return s.lockRecord(resource)
}
