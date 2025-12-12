package ragfs

import (
	"errors"
	"testing"
	"time"
)

// Test 1: Collector creation and initialization
func TestCollectorCreation(t *testing.T) {
	collector := NewCollector()

	if collector == nil {
		t.Fatal("NewCollector() returned nil")
	}

	// Verify mountedAt is set to approximately now
	now := time.Now()
	if collector.mountedAt.IsZero() {
		t.Error("mountedAt should be initialized to current time")
	}

	timeDiff := now.Sub(collector.mountedAt)
	if timeDiff < 0 || timeDiff > time.Second {
		t.Errorf("mountedAt time difference too large: %v", timeDiff)
	}

	// Verify all counters are zero
	snapshot := collector.Snapshot()
	if snapshot.CacheHits != 0 {
		t.Errorf("Initial cache hits should be 0, got %d", snapshot.CacheHits)
	}
	if snapshot.CacheMisses != 0 {
		t.Errorf("Initial cache misses should be 0, got %d", snapshot.CacheMisses)
	}
	if snapshot.BytesRead != 0 {
		t.Errorf("Initial bytes read should be 0, got %d", snapshot.BytesRead)
	}
	if snapshot.ErrorCount != 0 {
		t.Errorf("Initial error count should be 0, got %d", snapshot.ErrorCount)
	}
}

// Test 3: Recording cache operations
func TestCollectorCacheOperations(t *testing.T) {
	collector := NewCollector()

	// Record cache hit
	collector.RecordCacheHit()
	snapshot := collector.Snapshot()
	if snapshot.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", snapshot.CacheHits)
	}

	// Record multiple cache hits
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	snapshot = collector.Snapshot()
	if snapshot.CacheHits != 3 {
		t.Errorf("Expected 3 cache hits, got %d", snapshot.CacheHits)
	}

	// Record cache misses
	collector.RecordCacheMiss()
	snapshot = collector.Snapshot()
	if snapshot.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", snapshot.CacheMisses)
	}

	// Record evictions
	collector.RecordEviction()
	collector.RecordEviction()
	snapshot = collector.Snapshot()
	if snapshot.CacheEvictions != 2 {
		t.Errorf("Expected 2 cache evictions, got %d", snapshot.CacheEvictions)
	}

	// Update cache entries
	collector.UpdateCacheEntries(128)
	snapshot = collector.Snapshot()
	if snapshot.CacheEntries != 128 {
		t.Errorf("Expected 128 cache entries, got %d", snapshot.CacheEntries)
	}

	// Verify cache hit rate calculation
	// 3 hits, 1 miss = 3/4 = 0.75
	expectedRate := 0.75
	if snapshot.CacheHitRate != expectedRate {
		t.Errorf("Expected cache hit rate %.3f, got %.3f", expectedRate, snapshot.CacheHitRate)
	}
}

// Test 5: I/O tracking with latency
func TestCollectorIOTracking(t *testing.T) {
	collector := NewCollector()

	// Record a read operation
	readLatency := 5 * time.Millisecond
	collector.RecordRead(1024, readLatency)

	snapshot := collector.Snapshot()
	if snapshot.BytesRead != 1024 {
		t.Errorf("Expected 1024 bytes read, got %d", snapshot.BytesRead)
	}
	if snapshot.ReadOps != 1 {
		t.Errorf("Expected 1 read op, got %d", snapshot.ReadOps)
	}
	if snapshot.LastReadTime.IsZero() {
		t.Error("LastReadTime should be set")
	}

	// Average latency should be 5ms
	expectedAvgMs := 5.0
	if snapshot.AvgReadLatencyMs != expectedAvgMs {
		t.Errorf("Expected avg read latency %.3f ms, got %.3f ms", expectedAvgMs, snapshot.AvgReadLatencyMs)
	}

	// Record another read
	collector.RecordRead(2048, 10*time.Millisecond)
	snapshot = collector.Snapshot()

	// Total bytes: 1024 + 2048 = 3072
	if snapshot.BytesRead != 3072 {
		t.Errorf("Expected 3072 bytes read, got %d", snapshot.BytesRead)
	}
	// Total ops: 2
	if snapshot.ReadOps != 2 {
		t.Errorf("Expected 2 read ops, got %d", snapshot.ReadOps)
	}
	// Average latency: (5ms + 10ms) / 2 = 7.5ms
	expectedAvgMs = 7.5
	if snapshot.AvgReadLatencyMs != expectedAvgMs {
		t.Errorf("Expected avg read latency %.3f ms, got %.3f ms", expectedAvgMs, snapshot.AvgReadLatencyMs)
	}

	// Record write operations
	collector.RecordWrite(512, 2*time.Millisecond)
	snapshot = collector.Snapshot()

	if snapshot.BytesWritten != 512 {
		t.Errorf("Expected 512 bytes written, got %d", snapshot.BytesWritten)
	}
	if snapshot.WriteOps != 1 {
		t.Errorf("Expected 1 write op, got %d", snapshot.WriteOps)
	}
	if snapshot.AvgWriteLatencyMs != 2.0 {
		t.Errorf("Expected avg write latency 2.0 ms, got %.3f ms", snapshot.AvgWriteLatencyMs)
	}
}

// Test 7: Error tracking and sanitization
func TestCollectorErrorTracking(t *testing.T) {
	collector := NewCollector()

	// Record a simple error
	err1 := errors.New("connection timeout")
	collector.RecordError(err1, "TimeoutError")

	snapshot := collector.Snapshot()
	if snapshot.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", snapshot.ErrorCount)
	}
	if snapshot.LastErrorMessage != "connection timeout" {
		t.Errorf("Expected error message 'connection timeout', got '%s'", snapshot.LastErrorMessage)
	}
	if snapshot.LastErrorTime.IsZero() {
		t.Error("LastErrorTime should be set")
	}

	// Verify errors by type
	if snapshot.ErrorsByType["TimeoutError"] != 1 {
		t.Errorf("Expected 1 TimeoutError, got %d", snapshot.ErrorsByType["TimeoutError"])
	}

	// Verify recent errors list
	if len(snapshot.RecentErrors) != 1 {
		t.Errorf("Expected 1 recent error, got %d", len(snapshot.RecentErrors))
	}
	if snapshot.RecentErrors[0].Kind != "TimeoutError" {
		t.Errorf("Expected error kind 'TimeoutError', got '%s'", snapshot.RecentErrors[0].Kind)
	}

	// Test sanitization of sensitive data
	err2 := errors.New("failed to auth with api_key=sk-1234567890abcdef")
	collector.RecordError(err2, "AuthError")

	snapshot = collector.Snapshot()
	if snapshot.ErrorCount != 2 {
		t.Errorf("Expected 2 errors, got %d", snapshot.ErrorCount)
	}

	// Message should be sanitized
	if snapshot.LastErrorMessage == "failed to auth with api_key=sk-1234567890abcdef" {
		t.Error("Error message should be sanitized, but wasn't")
	}
	if snapshot.LastErrorMessage != "failed to auth with api_key=[REDACTED]" {
		t.Errorf("Expected sanitized message, got '%s'", snapshot.LastErrorMessage)
	}

	// Test sanitization of token
	err3 := errors.New("bearer token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
	collector.RecordError(err3, "AuthError")
	snapshot = collector.Snapshot()
	if snapshot.LastErrorMessage != "bearer token [REDACTED]" {
		t.Errorf("Expected sanitized token message, got '%s'", snapshot.LastErrorMessage)
	}

	// Test sanitization of password
	err4 := errors.New("login failed password=MySecretP@ss123")
	collector.RecordError(err4, "AuthError")
	snapshot = collector.Snapshot()
	if snapshot.LastErrorMessage != "login failed password=[REDACTED]" {
		t.Errorf("Expected sanitized password message, got '%s'", snapshot.LastErrorMessage)
	}
}

// Test 9: Snapshot immutability
func TestCollectorSnapshotImmutability(t *testing.T) {
	collector := NewCollector()

	// Record some data
	collector.RecordCacheHit()
	collector.RecordRead(1024, 5*time.Millisecond)

	// Take first snapshot
	snapshot1 := collector.Snapshot()

	// Modify collector
	collector.RecordCacheHit()
	collector.RecordRead(2048, 10*time.Millisecond)

	// Take second snapshot
	snapshot2 := collector.Snapshot()

	// First snapshot should remain unchanged
	if snapshot1.CacheHits != 1 {
		t.Errorf("Snapshot1 cache hits changed, expected 1, got %d", snapshot1.CacheHits)
	}
	if snapshot1.BytesRead != 1024 {
		t.Errorf("Snapshot1 bytes read changed, expected 1024, got %d", snapshot1.BytesRead)
	}

	// Second snapshot should have new values
	if snapshot2.CacheHits != 2 {
		t.Errorf("Snapshot2 expected 2 cache hits, got %d", snapshot2.CacheHits)
	}
	if snapshot2.BytesRead != 3072 {
		t.Errorf("Snapshot2 expected 3072 bytes read, got %d", snapshot2.BytesRead)
	}

	// Modifying snapshot1's map shouldn't affect collector
	snapshot1.ErrorsByType["Test"] = 999
	snapshot3 := collector.Snapshot()
	if _, exists := snapshot3.ErrorsByType["Test"]; exists {
		t.Error("Modifying snapshot map affected collector")
	}
}

// Test: Concurrent access safety
func TestCollectorConcurrency(t *testing.T) {
	collector := NewCollector()

	// Run concurrent operations
	done := make(chan bool)

	// Goroutine 1: Record cache hits
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordCacheHit()
		}
		done <- true
	}()

	// Goroutine 2: Record reads
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordRead(1024, time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Take snapshots
	go func() {
		for i := 0; i < 100; i++ {
			_ = collector.Snapshot()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify final state
	snapshot := collector.Snapshot()
	if snapshot.CacheHits != 100 {
		t.Errorf("Expected 100 cache hits, got %d", snapshot.CacheHits)
	}
	if snapshot.ReadOps != 100 {
		t.Errorf("Expected 100 read ops, got %d", snapshot.ReadOps)
	}
}
