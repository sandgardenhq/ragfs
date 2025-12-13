package ragfs

import (
	"regexp"
	"sync"
	"time"
)

// Collector tracks runtime metrics for a ragfs mount.
// All methods are thread-safe and can be called concurrently.
type Collector struct {
	mu sync.RWMutex

	// Timestamps
	mountedAt time.Time

	// Cache metrics
	cacheHits      int64
	cacheMisses    int64
	cacheEntries   int64
	cacheEvictions int64
	cacheCapacity  int64
	cacheTTL       time.Duration

	// I/O metrics
	bytesRead         int64
	bytesWritten      int64
	readOps           int64
	writeOps          int64
	totalReadLatency  time.Duration
	totalWriteLatency time.Duration
	lastReadTime      time.Time
	lastWriteTime     time.Time

	// Error tracking
	errorCount    int64
	lastError     error
	lastErrorTime time.Time
	errorsByType  map[string]int64
	recentErrors  []ErrorRecord // Ring buffer, max 10
}

// ErrorRecord represents a single error event.
type ErrorRecord struct {
	Timestamp time.Time
	Kind      string
	Message   string
}

// MetricsSnapshot is an immutable snapshot of metrics at a point in time.
type MetricsSnapshot struct {
	// Timestamps
	MountedAt time.Time

	// Cache metrics
	CacheHits       int64
	CacheMisses     int64
	CacheHitRate    float64
	CacheEntries    int64
	CacheEvictions  int64
	CacheCapacity   int64
	CacheTTLSeconds int64

	// I/O metrics
	BytesRead         int64
	BytesWritten      int64
	ReadOps           int64
	WriteOps          int64
	AvgReadLatencyMs  float64
	AvgWriteLatencyMs float64
	LastReadTime      time.Time
	LastWriteTime     time.Time

	// Error tracking
	ErrorCount       int64
	LastErrorTime    time.Time
	LastErrorMessage string
	ErrorsByType     map[string]int64
	RecentErrors     []ErrorRecord
}

// Regex patterns for sanitizing sensitive data
var (
	apiKeyPattern   = regexp.MustCompile(`(?i)(api[_-]?key|apikey)[\s:=]+[^\s]+`)
	tokenPattern    = regexp.MustCompile(`(?i)(token|bearer)(\s+token)?\s+[^\s]+`)
	passwordPattern = regexp.MustCompile(`(?i)(password|passwd|pwd)[\s:=]+[^\s]+`)
)

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		mountedAt:    time.Now(),
		errorsByType: make(map[string]int64),
		recentErrors: make([]ErrorRecord, 0, 10),
	}
}

// RecordCacheHit increments the cache hit counter.
func (c *Collector) RecordCacheHit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheHits++
}

// RecordCacheMiss increments the cache miss counter.
func (c *Collector) RecordCacheMiss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheMisses++
}

// RecordEviction increments the cache eviction counter.
func (c *Collector) RecordEviction() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheEvictions++
}

// UpdateCacheEntries sets the current cache entry count.
func (c *Collector) UpdateCacheEntries(count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheEntries = count
}

// RecordRead records a read operation with its size and latency.
func (c *Collector) RecordRead(bytes int64, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bytesRead += bytes
	c.readOps++
	c.totalReadLatency += latency
	c.lastReadTime = time.Now()
}

// RecordWrite records a write operation with its size and latency.
func (c *Collector) RecordWrite(bytes int64, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bytesWritten += bytes
	c.writeOps++
	c.totalWriteLatency += latency
	c.lastWriteTime = time.Now()
}

// RecordError records an error with its kind and message.
// The error message is sanitized to redact sensitive information.
func (c *Collector) RecordError(err error, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.errorCount++
	c.lastError = err
	c.lastErrorTime = time.Now()

	// Track errors by type
	c.errorsByType[kind]++

	// Sanitize error message
	message := err.Error()
	message = apiKeyPattern.ReplaceAllString(message, "$1=[REDACTED]")
	message = tokenPattern.ReplaceAllString(message, "$1$2 [REDACTED]")
	message = passwordPattern.ReplaceAllString(message, "$1=[REDACTED]")

	// Add to recent errors (ring buffer, max 10)
	record := ErrorRecord{
		Timestamp: c.lastErrorTime,
		Kind:      kind,
		Message:   message,
	}

	if len(c.recentErrors) >= 10 {
		// Shift and append
		c.recentErrors = append(c.recentErrors[1:], record)
	} else {
		c.recentErrors = append(c.recentErrors, record)
	}
}

// Snapshot returns an immutable snapshot of the current metrics.
func (c *Collector) Snapshot() MetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Calculate cache hit rate
	var hitRate float64
	totalCacheAccess := c.cacheHits + c.cacheMisses
	if totalCacheAccess > 0 {
		hitRate = float64(c.cacheHits) / float64(totalCacheAccess)
	}

	// Calculate average latencies
	var avgReadLatency float64
	if c.readOps > 0 {
		avgReadLatency = float64(c.totalReadLatency.Milliseconds()) / float64(c.readOps)
	}

	var avgWriteLatency float64
	if c.writeOps > 0 {
		avgWriteLatency = float64(c.totalWriteLatency.Milliseconds()) / float64(c.writeOps)
	}

	// Get last error message (sanitized)
	var lastErrorMsg string
	if c.lastError != nil {
		lastErrorMsg = c.lastError.Error()
		lastErrorMsg = apiKeyPattern.ReplaceAllString(lastErrorMsg, "$1=[REDACTED]")
		lastErrorMsg = tokenPattern.ReplaceAllString(lastErrorMsg, "$1$2 [REDACTED]")
		lastErrorMsg = passwordPattern.ReplaceAllString(lastErrorMsg, "$1=[REDACTED]")
	}

	// Copy errorsByType map to avoid mutation
	errorsByType := make(map[string]int64, len(c.errorsByType))
	for k, v := range c.errorsByType {
		errorsByType[k] = v
	}

	// Copy recent errors to avoid mutation
	recentErrors := make([]ErrorRecord, len(c.recentErrors))
	copy(recentErrors, c.recentErrors)

	return MetricsSnapshot{
		MountedAt: c.mountedAt,

		CacheHits:       c.cacheHits,
		CacheMisses:     c.cacheMisses,
		CacheHitRate:    hitRate,
		CacheEntries:    c.cacheEntries,
		CacheEvictions:  c.cacheEvictions,
		CacheCapacity:   c.cacheCapacity,
		CacheTTLSeconds: int64(c.cacheTTL.Seconds()),

		BytesRead:         c.bytesRead,
		BytesWritten:      c.bytesWritten,
		ReadOps:           c.readOps,
		WriteOps:          c.writeOps,
		AvgReadLatencyMs:  avgReadLatency,
		AvgWriteLatencyMs: avgWriteLatency,
		LastReadTime:      c.lastReadTime,
		LastWriteTime:     c.lastWriteTime,

		ErrorCount:       c.errorCount,
		LastErrorTime:    c.lastErrorTime,
		LastErrorMessage: lastErrorMsg,
		ErrorsByType:     errorsByType,
		RecentErrors:     recentErrors,
	}
}
