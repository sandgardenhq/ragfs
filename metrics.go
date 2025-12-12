package ragfs

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector tracks runtime statistics for a ragfs mount.
// All fields are thread-safe using either atomic operations or mutex protection.
type MetricsCollector struct {
	// Mount metadata (immutable after creation - thread-safe)
	mountedAt time.Time
	pid       int

	// Cache stats (delegate to existing CacheStats if available)
	// CacheStats already uses atomic operations internally - thread-safe
	cacheStats *CacheStats // nil if caching disabled

	// I/O stats - all atomic operations, lock-free reads and writes
	bytesRead     atomic.Int64
	bytesWritten  atomic.Int64
	readOps       atomic.Int64
	writeOps      atomic.Int64
	totalReadNs   atomic.Int64 // for avg latency
	totalWriteNs  atomic.Int64
	lastReadTime  atomic.Value // time.Time
	lastWriteTime atomic.Value // time.Time

	// Error tracking - uses mutex for complex data structures
	// Atomic counter for fast reads, mutex only for maps/slices
	errorsTotal  atomic.Int64
	errMu        sync.RWMutex
	errorsByType map[string]int64 // protected by errMu
	recentErrors []ErrorRecord    // protected by errMu (circular buffer, max 10)
	lastError    *ErrorRecord     // protected by errMu
}

// ErrorRecord represents a single error occurrence with sanitized details.
type ErrorRecord struct {
	Timestamp time.Time
	Kind      string
	Message   string // sanitized
}

// newMetricsCollector creates a new metrics collector initialized with current time and PID.
func newMetricsCollector(cacheStats *CacheStats) *MetricsCollector {
	mc := &MetricsCollector{
		mountedAt:    time.Now(),
		pid:          os.Getpid(),
		cacheStats:   cacheStats,
		errorsByType: make(map[string]int64),
		recentErrors: make([]ErrorRecord, 0, 10),
	}

	// Initialize atomic.Value fields with zero values
	mc.lastReadTime.Store(time.Time{})
	mc.lastWriteTime.Store(time.Time{})

	return mc
}

// RecordRead records a read operation with byte count and duration.
func (mc *MetricsCollector) RecordRead(bytes int64, duration time.Duration) {
	mc.bytesRead.Add(bytes)
	mc.readOps.Add(1)
	mc.totalReadNs.Add(int64(duration))
	mc.lastReadTime.Store(time.Now())
}

// RecordWrite records a write operation with byte count and duration.
func (mc *MetricsCollector) RecordWrite(bytes int64, duration time.Duration) {
	mc.bytesWritten.Add(bytes)
	mc.writeOps.Add(1)
	mc.totalWriteNs.Add(int64(duration))
	mc.lastWriteTime.Store(time.Now())
}

// RecordError records an error occurrence with sanitized message.
// The error message is sanitized to remove potential secrets before storage.
func (mc *MetricsCollector) RecordError(err error, kind string) {
	if err == nil {
		return
	}

	mc.errorsTotal.Add(1)

	sanitized := sanitizeErrorMessage(err.Error())
	record := ErrorRecord{
		Timestamp: time.Now(),
		Kind:      kind,
		Message:   sanitized,
	}

	mc.errMu.Lock()
	defer mc.errMu.Unlock()

	// Update error count by type
	mc.errorsByType[kind]++

	// Update last error
	mc.lastError = &record

	// Add to recent errors (circular buffer, max 10)
	if len(mc.recentErrors) >= 10 {
		// Remove oldest, add newest
		mc.recentErrors = append(mc.recentErrors[1:], record)
	} else {
		mc.recentErrors = append(mc.recentErrors, record)
	}
}

// Uptime returns the duration since the filesystem was mounted.
func (mc *MetricsCollector) Uptime() time.Duration {
	return time.Since(mc.mountedAt)
}

// AvgReadLatency returns the average read latency in milliseconds.
// Returns 0 if no reads have occurred.
func (mc *MetricsCollector) AvgReadLatency() float64 {
	ops := mc.readOps.Load()
	if ops == 0 {
		return 0
	}
	totalNs := mc.totalReadNs.Load()
	return float64(totalNs) / float64(ops) / 1e6 // convert to milliseconds
}

// AvgWriteLatency returns the average write latency in milliseconds.
// Returns 0 if no writes have occurred.
func (mc *MetricsCollector) AvgWriteLatency() float64 {
	ops := mc.writeOps.Load()
	if ops == 0 {
		return 0
	}
	totalNs := mc.totalWriteNs.Load()
	return float64(totalNs) / float64(ops) / 1e6 // convert to milliseconds
}

// GetErrorsByType returns a copy of the errors-by-type map.
// This is a snapshot and safe to read concurrently.
func (mc *MetricsCollector) GetErrorsByType() map[string]int64 {
	mc.errMu.RLock()
	defer mc.errMu.RUnlock()

	result := make(map[string]int64, len(mc.errorsByType))
	for k, v := range mc.errorsByType {
		result[k] = v
	}
	return result
}

// GetRecentErrors returns a copy of recent errors.
// This is a snapshot and safe to read concurrently.
func (mc *MetricsCollector) GetRecentErrors() []ErrorRecord {
	mc.errMu.RLock()
	defer mc.errMu.RUnlock()

	result := make([]ErrorRecord, len(mc.recentErrors))
	copy(result, mc.recentErrors)
	return result
}

// GetLastError returns a copy of the last error, or nil if no errors have occurred.
func (mc *MetricsCollector) GetLastError() *ErrorRecord {
	mc.errMu.RLock()
	defer mc.errMu.RUnlock()

	if mc.lastError == nil {
		return nil
	}

	// Return a copy to prevent mutation
	result := *mc.lastError
	return &result
}

// sanitizeErrorMessage removes potential secrets from error messages.
// It replaces patterns that look like API keys, tokens, passwords, etc.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b[A-Za-z0-9_-]{32,}\b`),                    // API keys (long alphanumeric strings)
	regexp.MustCompile(`(?i)token[=:\s]+[A-Za-z0-9_-]+`),            // token=xyz
	regexp.MustCompile(`(?i)key[=:\s]+[A-Za-z0-9_-]+`),              // key=xyz
	regexp.MustCompile(`(?i)password[=:\s]+\S+`),                    // password=xyz
	regexp.MustCompile(`(?i)secret[=:\s]+\S+`),                      // secret=xyz
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9_-]+`),                   // Bearer tokens
	regexp.MustCompile(`(?i)authorization[=:\s]+[A-Za-z0-9_\-\.]+`), // authorization headers
}

func sanitizeErrorMessage(msg string) string {
	sanitized := msg
	for _, pattern := range secretPatterns {
		sanitized = pattern.ReplaceAllString(sanitized, "[REDACTED]")
	}
	return sanitized
}

// GetLastReadTime returns the timestamp of the last read operation.
func (mc *MetricsCollector) GetLastReadTime() time.Time {
	t, ok := mc.lastReadTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return t
}

// GetLastWriteTime returns the timestamp of the last write operation.
func (mc *MetricsCollector) GetLastWriteTime() time.Time {
	t, ok := mc.lastWriteTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return t
}

// FormatTimestamp formats a time as RFC3339 or returns "never" if zero.
func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

// FormatDuration formats a duration in human-readable form.
func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	} else if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/1000)
	} else if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
	} else if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.2fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.2fh", d.Hours())
	}
}
