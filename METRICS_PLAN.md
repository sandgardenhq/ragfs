# Metrics Filesystem Implementation Plan

## Overview
Implement a read-only `/_metrics/` filesystem tree that exposes runtime statistics for any ragfs mount. This feature provides standardized observability without requiring implementers to add custom metrics code.

## Architecture

### Components

1. **MetricsCollector** (`metrics.go`)
   - Central in-process collector that tracks all runtime statistics
   - Thread-safe with atomic counters and mutexes where needed
   - Initialized automatically when FS is created
   - Tracks: cache stats, I/O stats, system stats, errors

2. **MetricsFS** (`metrics_fs.go`)
   - Virtual filesystem that exposes collector data as files
   - Registered as a special handler for `/_metrics/**` pattern
   - Generates content on-demand when files are read
   - Provides both individual stat files and summary.md files

3. **Integration with ragfs.FS**
   - FS struct gets a `metrics *MetricsCollector` field
   - All I/O operations update metrics (Open, ReadDir, etc.)
   - Cache operations already tracked via existing stats
   - Error tracking added to handler invocations

### File Tree Structure

```
/_metrics/
  ├─ summary.md                    # Top-level summary (Markdown + JSON)
  ├─ version.txt                   # Schema version: "v1"
  ├─ cache/
  │   ├─ summary.md
  │   ├─ cache_hit_count
  │   ├─ cache_miss_count
  │   ├─ cache_hit_rate
  │   ├─ cache_entries
  │   ├─ cache_evictions
  │   ├─ cache_capacity_bytes
  │   └─ cache_default_ttl_seconds
  ├─ io/
  │   ├─ summary.md
  │   ├─ bytes_read_total
  │   ├─ bytes_written_total
  │   ├─ read_ops_total
  │   ├─ write_ops_total
  │   ├─ avg_read_latency_ms
  │   ├─ avg_write_latency_ms
  │   ├─ last_read_time
  │   └─ last_write_time
  ├─ system/
  │   ├─ summary.md
  │   ├─ uptime_seconds
  │   ├─ mounted_at
  │   ├─ process_pid
  │   ├─ memory_rss_bytes
  │   ├─ cpu_seconds_total
  │   ├─ goroutines
  │   └─ open_fds
  └─ errors/
      ├─ summary.md
      ├─ errors_total
      ├─ last_error_time
      ├─ last_error_message
      ├─ errors_by_type.json
      └─ recent_errors.json
```

## Implementation Steps

### Phase 1: Core Metrics Collection

1. **Create `metrics.go`**
   - Define `MetricsCollector` struct with all counters
   - Implement thread-safe increment/update methods
   - Add methods to get current values for rendering
   - Track mount time for uptime calculation

2. **Integrate with FS struct**
   - Add `metrics *MetricsCollector` field to `ragfs.FS`
   - Initialize in `New()` function
   - Add instrumentation to existing methods:
     - `Open()`: track I/O, latency, errors
     - `ReadDir()`: track I/O, errors
     - Cache hits/misses (already tracked, reuse)
     - Handler errors

### Phase 2: Metrics Filesystem

3. **Create `metrics_fs.go`**
   - Implement handler function for `/_metrics/**` pattern
   - Parse paths to determine which file/dir to serve
   - Generate content on-demand:
     - Single-value files: format as "value\n"
     - JSON files: marshal from collector data
     - summary.md files: template-based generation
     - Directory listings: return DirEntry slice

4. **Register metrics handler**
   - In `New()`, register `/_metrics` pattern FIRST (highest priority)
   - Ensure user handlers cannot override `/_metrics/*`

### Phase 3: Summary Generation

5. **Create summary templates**
   - Top-level summary template with major stats table
   - Group summary templates (cache, io, system, errors)
   - Include RFC3339 timestamps
   - Include JSON code blocks for machine parsing
   - Ensure Markdown validity

6. **Implement summary generators**
   - `generateTopLevelSummary(collector) string`
   - `generateCacheSummary(collector) string`
   - `generateIOSummary(collector) string`
   - `generateSystemSummary(collector) string`
   - `generateErrorsSummary(collector) string`

### Phase 4: System Metrics

7. **Implement system metrics collection**
   - Use `runtime.ReadMemStats()` for memory
   - Use `runtime.NumGoroutine()` for goroutines
   - Use `os.Getpid()` for PID
   - CPU time: track with `time.Since(start)` or use `/proc` on Linux
   - Open FDs: platform-specific (may skip for MVP)

### Phase 5: Error Tracking

8. **Implement error tracking**
   - Add `RecordError(err error, kind string)` to collector
   - Store recent errors (circular buffer, last 10)
   - Track error counts by type
   - Sanitize error messages (replace secrets with "[REDACTED]")
   - Call from handler error paths

### Phase 6: Testing

9. **Unit tests**
   - `metrics_test.go`: Test collector thread-safety, accuracy
   - `metrics_fs_test.go`: Test file generation, path routing
   - Test summary Markdown parsing, JSON validity
   - Test RFC3339 timestamp format

10. **Integration tests**
    - Test full `/_metrics/` tree is present
    - Test reading each individual file
    - Test summary.md files render correctly
    - Test stats update when operations performed
    - Test error tracking and sanitization

### Phase 7: Documentation

11. **Update README.md**
    - Add "Observability" section
    - Document `/_metrics/` feature
    - Provide example of reading summary.md
    - Show how to parse JSON snippets

12. **Add example**
    - Create `examples/metrics/` demonstrating metrics usage
    - Show reading various metrics files
    - Show parsing summary JSON

## Data Structures

### MetricsCollector

```go
type MetricsCollector struct {
	// Mount metadata (immutable after creation - thread-safe)
	mountedAt time.Time
	pid       int

	// Cache stats (delegate to existing CacheStats if available)
	// CacheStats already uses atomic operations internally - thread-safe
	cacheStats *CacheStats  // nil if caching disabled

	// I/O stats - all atomic operations, lock-free reads and writes
	bytesRead     atomic.Int64
	bytesWritten  atomic.Int64
	readOps       atomic.Int64
	writeOps      atomic.Int64
	totalReadNs   atomic.Int64  // for avg latency
	totalWriteNs  atomic.Int64
	lastReadTime  atomic.Value  // time.Time
	lastWriteTime atomic.Value  // time.Time

	// Error tracking - uses mutex for complex data structures
	// Atomic counter for fast reads, mutex only for maps/slices
	errorsTotal   atomic.Int64
	errMu         sync.RWMutex
	errorsByType  map[string]int64  // protected by errMu
	recentErrors  []ErrorRecord     // protected by errMu (circular buffer, max 10)
	lastError     *ErrorRecord      // protected by errMu
}

type ErrorRecord struct {
	Timestamp time.Time
	Kind      string
	Message   string // sanitized
}
```

## Testing Strategy

### Unit Tests
- MetricsCollector: concurrent updates, accurate counting
- Summary generation: valid Markdown, valid JSON
- Path routing: correct file mapping
- Error sanitization

### Integration Tests
- Full tree traversal
- Stats accuracy after operations
- Cache stats integration
- Error tracking flow

## Edge Cases

1. **Cache disabled**: cache metrics should show zeros or N/A
2. **No errors**: error metrics show zeros, empty arrays
3. **Concurrent reads**: metrics generation is thread-safe
4. **Long-running mounts**: uptime doesn't overflow (use int64 seconds)
5. **Error message secrets**: sanitize API keys, tokens, passwords

## Performance Considerations

- Metrics collection uses atomic operations (minimal overhead)
- Summary generation is on-demand (not cached)
- Error tracking limited to last 10 errors (bounded memory)
- No background goroutines needed

## Thread Safety Guarantees

### Lock-Free Operations (I/O Metrics)
- All I/O counters use `atomic.Int64` for lock-free increments
- `lastReadTime` and `lastWriteTime` use `atomic.Value` for lock-free reads/writes
- No mutex contention on hot paths (Open, ReadDir operations)

### Mutex-Protected Operations (Error Tracking)
- Total error count uses `atomic.Int64` for fast reads without locking
- Complex data (maps, slices) protected by `sync.RWMutex`:
  - `errorsByType` map (write lock on record, read lock on summary)
  - `recentErrors` slice (write lock on record, read lock on summary)
  - `lastError` pointer (write lock on record, read lock on summary)
- Read operations use `RLock()` to allow concurrent summary generation
- Write operations use `Lock()` only when recording new errors

### Immutable Data
- `mountedAt` and `pid` set once during initialization (thread-safe)
- `cacheStats` pointer set once during initialization, underlying CacheStats uses atomics

### Summary Generation
- Summary generation takes read locks only
- Multiple concurrent reads of `/_metrics/**` are safe
- No lock held during string formatting (minimizes contention)

## Acceptance Criteria

- [ ] All required `/_metrics/` paths exist and return valid content
- [ ] summary.md files are valid Markdown with valid JSON blocks
- [ ] Single-value stat files return correct format: "value\n"
- [ ] Stats update when operations are performed
- [ ] Error messages are sanitized
- [ ] All tests pass
- [ ] Documentation updated
- [ ] Example demonstrates usage

## Non-Goals (Out of Scope)

- Prometheus exposition format (future enhancement)
- Per-route metrics (future enhancement)
- Metrics retention/history (current values only)
- External metrics push (pull-only via filesystem reads)
