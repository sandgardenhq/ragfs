package ragfs

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMetricsCollector_RecordRead tests recording read operations.
func TestMetricsCollector_RecordRead(t *testing.T) {
	mc := newMetricsCollector(nil)

	// Record a read operation
	mc.RecordRead(100, 5*time.Millisecond)

	if got := mc.bytesRead.Load(); got != 100 {
		t.Errorf("bytesRead = %d, want 100", got)
	}

	if got := mc.readOps.Load(); got != 1 {
		t.Errorf("readOps = %d, want 1", got)
	}

	if got := mc.AvgReadLatency(); got < 4.0 || got > 6.0 {
		t.Errorf("AvgReadLatency = %.2f ms, want ~5.0 ms", got)
	}

	lastRead := mc.GetLastReadTime()
	if lastRead.IsZero() {
		t.Error("lastReadTime should not be zero")
	}
}

// TestMetricsCollector_RecordWrite tests recording write operations.
func TestMetricsCollector_RecordWrite(t *testing.T) {
	mc := newMetricsCollector(nil)

	// Record a write operation
	mc.RecordWrite(200, 10*time.Millisecond)

	if got := mc.bytesWritten.Load(); got != 200 {
		t.Errorf("bytesWritten = %d, want 200", got)
	}

	if got := mc.writeOps.Load(); got != 1 {
		t.Errorf("writeOps = %d, want 1", got)
	}

	if got := mc.AvgWriteLatency(); got < 9.0 || got > 11.0 {
		t.Errorf("AvgWriteLatency = %.2f ms, want ~10.0 ms", got)
	}

	lastWrite := mc.GetLastWriteTime()
	if lastWrite.IsZero() {
		t.Error("lastWriteTime should not be zero")
	}
}

// TestMetricsCollector_RecordError tests error recording with sanitization.
func TestMetricsCollector_RecordError(t *testing.T) {
	mc := newMetricsCollector(nil)

	// Record a simple error
	err := fmt.Errorf("connection failed")
	mc.RecordError(err, "network")

	if got := mc.errorsTotal.Load(); got != 1 {
		t.Errorf("errorsTotal = %d, want 1", got)
	}

	errorsByType := mc.GetErrorsByType()
	if count, ok := errorsByType["network"]; !ok || count != 1 {
		t.Errorf("errorsByType[network] = %d, want 1", count)
	}

	lastErr := mc.GetLastError()
	if lastErr == nil {
		t.Fatal("GetLastError() = nil, want error record")
	}
	if lastErr.Kind != "network" {
		t.Errorf("lastErr.Kind = %s, want network", lastErr.Kind)
	}
	if lastErr.Message != "connection failed" {
		t.Errorf("lastErr.Message = %s, want connection failed", lastErr.Message)
	}

	recentErrors := mc.GetRecentErrors()
	if len(recentErrors) != 1 {
		t.Errorf("len(recentErrors) = %d, want 1", len(recentErrors))
	}
}

// TestMetricsCollector_ErrorSanitization tests that secrets are removed from errors.
func TestMetricsCollector_ErrorSanitization(t *testing.T) {
	mc := newMetricsCollector(nil)

	tests := []struct {
		name     string
		errMsg   string
		wantSafe bool // true if should be redacted
	}{
		{
			name:     "API key in error",
			errMsg:   "auth failed with key sk_test_abcdefghijklmnopqrstuvwxyz123456",
			wantSafe: true,
		},
		{
			name:     "Token in error",
			errMsg:   "token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantSafe: true,
		},
		{
			name:     "Password in error",
			errMsg:   "password=mysecret123",
			wantSafe: true,
		},
		{
			name:     "Bearer token",
			errMsg:   "Authorization: Bearer abc123xyz456",
			wantSafe: true,
		},
		{
			name:     "Normal error message",
			errMsg:   "connection timeout after 30s",
			wantSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errMsg)
			mc.RecordError(err, "test")

			lastErr := mc.GetLastError()
			if lastErr == nil {
				t.Fatal("GetLastError() = nil")
			}

			if tt.wantSafe {
				if lastErr.Message == tt.errMsg {
					t.Errorf("Error not sanitized: %s", lastErr.Message)
				}
				if lastErr.Message != "[REDACTED]" && lastErr.Message == tt.errMsg {
					t.Errorf("Expected redaction but got: %s", lastErr.Message)
				}
			} else {
				if lastErr.Message != tt.errMsg {
					t.Errorf("Normal message changed: got %s, want %s", lastErr.Message, tt.errMsg)
				}
			}
		})
	}
}

// TestMetricsCollector_RecentErrorsCircularBuffer tests the circular buffer for recent errors.
func TestMetricsCollector_RecentErrorsCircularBuffer(t *testing.T) {
	mc := newMetricsCollector(nil)

	// Record 15 errors (buffer size is 10)
	for i := 1; i <= 15; i++ {
		err := fmt.Errorf("error %d", i)
		mc.RecordError(err, "test")
	}

	if got := mc.errorsTotal.Load(); got != 15 {
		t.Errorf("errorsTotal = %d, want 15", got)
	}

	recentErrors := mc.GetRecentErrors()
	if len(recentErrors) != 10 {
		t.Errorf("len(recentErrors) = %d, want 10 (circular buffer limit)", len(recentErrors))
	}

	// First error in buffer should be "error 6" (errors 1-5 were evicted)
	if recentErrors[0].Message != "error 6" {
		t.Errorf("oldest error = %s, want 'error 6'", recentErrors[0].Message)
	}

	// Last error should be "error 15"
	if recentErrors[9].Message != "error 15" {
		t.Errorf("newest error = %s, want 'error 15'", recentErrors[9].Message)
	}
}

// TestMetricsCollector_Uptime tests uptime calculation.
func TestMetricsCollector_Uptime(t *testing.T) {
	mc := newMetricsCollector(nil)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	uptime := mc.Uptime()
	if uptime < 10*time.Millisecond {
		t.Errorf("Uptime = %s, want at least 10ms", uptime)
	}
	if uptime > 1*time.Second {
		t.Errorf("Uptime = %s, want less than 1s", uptime)
	}
}

// TestMetricsCollector_AvgLatency tests average latency calculation.
func TestMetricsCollector_AvgLatency(t *testing.T) {
	mc := newMetricsCollector(nil)

	// Record multiple reads with different latencies
	mc.RecordRead(100, 10*time.Millisecond)
	mc.RecordRead(100, 20*time.Millisecond)
	mc.RecordRead(100, 30*time.Millisecond)

	// Average should be 20ms
	avgLatency := mc.AvgReadLatency()
	if avgLatency < 19.0 || avgLatency > 21.0 {
		t.Errorf("AvgReadLatency = %.2f ms, want ~20.0 ms", avgLatency)
	}

	// Test with no operations
	mc2 := newMetricsCollector(nil)
	if got := mc2.AvgReadLatency(); got != 0 {
		t.Errorf("AvgReadLatency with no ops = %.2f, want 0", got)
	}
	if got := mc2.AvgWriteLatency(); got != 0 {
		t.Errorf("AvgWriteLatency with no ops = %.2f, want 0", got)
	}
}

// TestMetricsCollector_ConcurrentReads tests thread-safety of read recording.
func TestMetricsCollector_ConcurrentReads(t *testing.T) {
	mc := newMetricsCollector(nil)

	const numGoroutines = 100
	const opsPerGoroutine = 1000
	const expectedTotal = numGoroutines * opsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Spawn multiple goroutines recording reads concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				mc.RecordRead(1, 1*time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// Verify counts are accurate
	if got := mc.readOps.Load(); got != expectedTotal {
		t.Errorf("readOps = %d, want %d", got, expectedTotal)
	}
	if got := mc.bytesRead.Load(); got != expectedTotal {
		t.Errorf("bytesRead = %d, want %d", got, expectedTotal)
	}
}

// TestMetricsCollector_ConcurrentWrites tests thread-safety of write recording.
func TestMetricsCollector_ConcurrentWrites(t *testing.T) {
	mc := newMetricsCollector(nil)

	const numGoroutines = 100
	const opsPerGoroutine = 1000
	const expectedTotal = numGoroutines * opsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Spawn multiple goroutines recording writes concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				mc.RecordWrite(1, 1*time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// Verify counts are accurate
	if got := mc.writeOps.Load(); got != expectedTotal {
		t.Errorf("writeOps = %d, want %d", got, expectedTotal)
	}
	if got := mc.bytesWritten.Load(); got != expectedTotal {
		t.Errorf("bytesWritten = %d, want %d", got, expectedTotal)
	}
}

// TestMetricsCollector_ConcurrentErrors tests thread-safety of error recording.
func TestMetricsCollector_ConcurrentErrors(t *testing.T) {
	mc := newMetricsCollector(nil)

	const numGoroutines = 50
	const errorsPerGoroutine = 10
	const expectedTotal = numGoroutines * errorsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Spawn multiple goroutines recording errors concurrently
	for i := 0; i < numGoroutines; i++ {
		goroutineID := i
		go func() {
			defer wg.Done()
			for j := 0; j < errorsPerGoroutine; j++ {
				err := fmt.Errorf("error from goroutine %d", goroutineID)
				mc.RecordError(err, fmt.Sprintf("type-%d", goroutineID%5))
			}
		}()
	}

	wg.Wait()

	// Verify total count
	if got := mc.errorsTotal.Load(); got != expectedTotal {
		t.Errorf("errorsTotal = %d, want %d", got, expectedTotal)
	}

	// Verify errors by type map integrity
	errorsByType := mc.GetErrorsByType()
	totalByType := int64(0)
	for _, count := range errorsByType {
		totalByType += count
	}
	if totalByType != expectedTotal {
		t.Errorf("sum of errorsByType = %d, want %d", totalByType, expectedTotal)
	}

	// Verify recent errors buffer is intact (max 10 entries)
	recentErrors := mc.GetRecentErrors()
	if len(recentErrors) > 10 {
		t.Errorf("len(recentErrors) = %d, want <= 10", len(recentErrors))
	}

	// Verify last error is set
	lastErr := mc.GetLastError()
	if lastErr == nil {
		t.Error("GetLastError() = nil, want error record")
	}
}

// TestFormatDuration tests duration formatting.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{10 * time.Nanosecond, "10ns"},
		{1500 * time.Nanosecond, "1.50µs"},
		{2500 * time.Microsecond, "2.50ms"},
		{1500 * time.Millisecond, "1.50s"},
		{90 * time.Second, "1.50m"},
		{3 * time.Hour, "3.00h"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%s) = %s, want %s", tt.duration, got, tt.want)
			}
		})
	}
}

// TestFormatTimestamp tests timestamp formatting.
func TestFormatTimestamp(t *testing.T) {
	// Test zero time
	if got := formatTimestamp(time.Time{}); got != "never" {
		t.Errorf("formatTimestamp(zero) = %s, want 'never'", got)
	}

	// Test non-zero time
	ts := time.Date(2025, 12, 12, 10, 30, 45, 0, time.UTC)
	got := formatTimestamp(ts)
	want := "2025-12-12T10:30:45Z"
	if got != want {
		t.Errorf("formatTimestamp() = %s, want %s", got, want)
	}
}
