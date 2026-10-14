package main

import (
	"path/filepath"
	"testing"
	"time"
)

func newLockTestStorage(t *testing.T) *Storage {
	t.Helper()
	store, err := NewStorage(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestLockAcquireFreeResource(t *testing.T) {
	store := newLockTestStorage(t)

	acquired, rec, err := store.LockAcquire("repo/a", "owner-1", 5*time.Second)
	if err != nil {
		t.Fatalf("LockAcquire() error = %v", err)
	}
	if !acquired {
		t.Fatal("LockAcquire() = false, want true")
	}
	if rec == nil || rec.Owner != "owner-1" {
		t.Fatalf("LockAcquire() record = %+v, want owner owner-1", rec)
	}
}

func TestLockAcquireAlreadyHeld(t *testing.T) {
	store := newLockTestStorage(t)

	if _, _, err := store.LockAcquire("repo/a", "owner-1", 5*time.Second); err != nil {
		t.Fatalf("first LockAcquire() error = %v", err)
	}

	acquired, rec, err := store.LockAcquire("repo/a", "owner-2", 5*time.Second)
	if err != nil {
		t.Fatalf("second LockAcquire() error = %v", err)
	}
	if acquired {
		t.Fatal("LockAcquire() by different owner = true, want false")
	}
	if rec == nil || rec.Owner != "owner-1" {
		t.Fatalf("LockAcquire() record = %+v, want owner owner-1", rec)
	}
}

func TestLockReacquireBySameOwner(t *testing.T) {
	store := newLockTestStorage(t)

	first, rec1, err := store.LockAcquire("repo/a", "owner-1", 2*time.Second)
	if err != nil || !first {
		t.Fatalf("first acquire failed: acquired=%v err=%v", first, err)
	}

	time.Sleep(10 * time.Millisecond)
	second, rec2, err := store.LockAcquire("repo/a", "owner-1", 5*time.Second)
	if err != nil || !second {
		t.Fatalf("reacquire failed: acquired=%v err=%v", second, err)
	}
	if rec2.ExpiresAt <= rec1.ExpiresAt {
		t.Fatalf("reacquire did not extend expires_at: %s <= %s", rec2.ExpiresAt, rec1.ExpiresAt)
	}
}

func TestLockRelease(t *testing.T) {
	store := newLockTestStorage(t)

	if _, _, err := store.LockAcquire("repo/a", "owner-1", 5*time.Second); err != nil {
		t.Fatalf("LockAcquire() error = %v", err)
	}

	released, err := store.LockRelease("repo/a", "owner-2")
	if err != nil {
		t.Fatalf("LockRelease() by wrong owner error = %v", err)
	}
	if released {
		t.Fatal("LockRelease() by wrong owner = true, want false")
	}

	status, err := store.LockStatus("repo/a")
	if err != nil {
		t.Fatalf("LockStatus() error = %v", err)
	}
	if status == nil {
		t.Fatal("lock disappeared after failed release")
	}

	released, err = store.LockRelease("repo/a", "owner-1")
	if err != nil || !released {
		t.Fatalf("LockRelease() by owner = %v err=%v, want true", released, err)
	}

	status, err = store.LockStatus("repo/a")
	if err != nil {
		t.Fatalf("LockStatus() after release error = %v", err)
	}
	if status != nil {
		t.Fatalf("LockStatus() after release = %+v, want nil", status)
	}
}

func TestLockExtend(t *testing.T) {
	store := newLockTestStorage(t)

	if _, _, err := store.LockAcquire("repo/a", "owner-1", 2*time.Second); err != nil {
		t.Fatalf("LockAcquire() error = %v", err)
	}

	extended, rec, err := store.LockExtend("repo/a", "owner-2", 10*time.Second)
	if err != nil {
		t.Fatalf("LockExtend() by wrong owner error = %v", err)
	}
	if extended {
		t.Fatal("LockExtend() by wrong owner = true, want false")
	}

	extended, rec, err = store.LockExtend("repo/a", "owner-1", 10*time.Second)
	if err != nil || !extended {
		t.Fatalf("LockExtend() by owner = %v err=%v rec=%+v, want true", extended, err, rec)
	}

	// Status should reflect the new expiry.
	status, err := store.LockStatus("repo/a")
	if err != nil {
		t.Fatalf("LockStatus() error = %v", err)
	}
	if status == nil || status.ExpiresAt != rec.ExpiresAt {
		t.Fatalf("LockStatus() = %+v, want expiry %s", status, rec.ExpiresAt)
	}
}

func TestLockExpires(t *testing.T) {
	store := newLockTestStorage(t)

	if _, _, err := store.LockAcquire("repo/a", "owner-1", 100*time.Millisecond); err != nil {
		t.Fatalf("LockAcquire() error = %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	acquired, rec, err := store.LockAcquire("repo/a", "owner-2", 5*time.Second)
	if err != nil {
		t.Fatalf("LockAcquire() after expiry error = %v", err)
	}
	if !acquired {
		t.Fatal("LockAcquire() after expiry = false, want true")
	}
	if rec == nil || rec.Owner != "owner-2" {
		t.Fatalf("LockAcquire() after expiry record = %+v, want owner owner-2", rec)
	}

	status, err := store.LockStatus("repo/a")
	if err != nil {
		t.Fatalf("LockStatus() error = %v", err)
	}
	if status == nil || status.Owner != "owner-2" {
		t.Fatalf("LockStatus() = %+v, want owner owner-2", status)
	}
}

func TestLockValidation(t *testing.T) {
	store := newLockTestStorage(t)

	if _, _, err := store.LockAcquire("", "owner", time.Second); err == nil {
		t.Fatal("LockAcquire() with empty resource did not error")
	}
	if _, _, err := store.LockAcquire("resource", "", time.Second); err == nil {
		t.Fatal("LockAcquire() with empty owner did not error")
	}
	if _, _, err := store.LockAcquire("resource", "owner", 0); err == nil {
		t.Fatal("LockAcquire() with zero ttl did not error")
	}

	if _, err := store.LockRelease("", "owner"); err == nil {
		t.Fatal("LockRelease() with empty resource did not error")
	}
	if _, _, err := store.LockExtend("resource", "owner", 0); err == nil {
		t.Fatal("LockExtend() with zero ttl did not error")
	}
	if _, err := store.LockStatus(""); err == nil {
		t.Fatal("LockStatus() with empty resource did not error")
	}
}

func TestLockIndependentResources(t *testing.T) {
	store := newLockTestStorage(t)

	if _, _, err := store.LockAcquire("repo/a", "owner-1", 5*time.Second); err != nil {
		t.Fatalf("LockAcquire(repo/a) error = %v", err)
	}
	acquired, _, err := store.LockAcquire("repo/b", "owner-1", 5*time.Second)
	if err != nil || !acquired {
		t.Fatalf("LockAcquire(repo/b) = %v err=%v, want true", acquired, err)
	}
	acquired, _, err = store.LockAcquire("repo/b", "owner-2", 5*time.Second)
	if err != nil || acquired {
		t.Fatalf("LockAcquire(repo/b) by different owner = %v err=%v, want false", acquired, err)
	}
}
