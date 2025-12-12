# Implementation Plan: Mount-Level Metrics Filesystem (`/_metrics/`)

## Overview
Implement a standardized `/_metrics/` read-only filesystem tree at the root of every ragfs mount, exposing runtime metrics automatically. This includes cache stats, I/O metrics, system metrics, and error tracking.

---

## Architecture & Design Decisions

### Design Decisions (Confirmed)
1. **Metrics Scope:** Collect metrics from the current process (PID, memory, goroutines, FDs)
2. **Cache Metrics Source:** Duplicate tracking in new Collector (supports both LRU and future BoltDB cache)
3. **Metrics Enable/Disable:** Always enabled (per spec: "MUST expose")
4. **Error Message Sanitization:** Regex-based sanitization to redact tokens, passwords, API keys
5. **Average Latency Calculation:** Simple cumulative average: `total_latency / operation_count`

---

## Architecture Components

### 1. **Metrics Collector** (`metrics.go`)
Create a thread-safe in-process collector that tracks all mount-level statistics:

**Data Structure:**
```go
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
    bytesRead       int64
    bytesWritten    int64
    readOps         int64
    writeOps        int64
    totalReadLatency  time.Duration
    totalWriteLatency time.Duration
    lastReadTime      time.Time
    lastWriteTime     time.Time

    // Error tracking
    errorCount     int64
    lastError      error
    lastErrorTime  time.Time
    errorsByType   map[string]int64
    recentErrors   []ErrorRecord  // Ring buffer, max 10
}

type ErrorRecord struct {
    Timestamp time.Time
    Kind      string
    Message   string
}
```

**Methods:**
- `RecordCacheHit()`, `RecordCacheMiss()`, `RecordEviction()`, `UpdateCacheEntries(count int64)`
- `RecordRead(bytes int64, latency time.Duration)`
- `RecordWrite(bytes int64, latency time.Duration)`
- `RecordError(err error, kind string)`
- `Snapshot() MetricsSnapshot` - Returns immutable snapshot of current metrics

---

### 2. **Metrics Filesystem** (`metrics_fs.go`)
Implement a handler that serves the `/_metrics/` tree:

**Path Routing:**
- `/_metrics/summary.md` → Top-level summary with table and JSON
- `/_metrics/version.txt` → "v1"
- `/_metrics/cache/` → Cache group
- `/_metrics/io/` → I/O group
- `/_metrics/system/` → System group
- `/_metrics/errors/` → Error group

**Implementation approach:**
- Add special check in `ragfs.Open()`: if path starts with `/_metrics/`, route to metrics handler
- Metrics handler generates content **on-demand** when files are read
- Use existing DirEntry interface for consistency

---

### 3. **Integration Points**

**In `ragfs.go`:**
- Add `collector *Collector` field to `FS` struct
- Initialize collector in `New()`
- Instrument existing code:
  - Cache hits/misses → duplicate tracking in collector (supports future BoltDB cache)
  - I/O operations → wrap in timing/counting logic
  - Errors → wrap error returns with `collector.RecordError()`

**In `fuse.go`:**
- System metrics (PID, memory, goroutines, FDs) collected via `runtime` package
- Uptime calculated from `mountedAt` timestamp

---

## Implementation Steps (Test-Driven Development)

### Phase 1: Metrics Collector
1. Write test for `Collector` creation and initialization
2. Implement `Collector` struct with thread-safe methods
3. Write tests for recording cache operations
4. Implement `RecordCacheHit()`, `RecordCacheMiss()`, `RecordEviction()`, `UpdateCacheEntries()`
5. Write tests for I/O tracking with latency
6. Implement `RecordRead()`, `RecordWrite()`
7. Write tests for error tracking and sanitization (redact API keys, tokens, passwords)
8. Implement `RecordError()` with regex-based message sanitization
9. Write test for `Snapshot()` immutability
10. Implement `Snapshot()` method with immutable snapshot struct

### Phase 2: Summary Generation
1. Write test for top-level `summary.md` format (Markdown table + JSON block)
2. Implement `GenerateSummaryMarkdown(snapshot)` function
3. Write test for group `summary.md` format (cache, io, system, errors)
4. Implement group-specific summary generators
5. Write test for RFC3339 timestamp formatting
6. Write test for numeric formatting (integers vs floats with 3 decimal places)

### Phase 3: Metrics Filesystem Handler
1. Write test for `/_metrics/summary.md` file read
2. Implement metrics handler routing in `Open()`
3. Write test for `/_metrics/version.txt`
4. Implement version file handler
5. Write tests for single-value stat files (e.g., `cache_hit_count`)
6. Implement single-value file handlers
7. Write test for JSON files (`errors_by_type.json`, `recent_errors.json`)
8. Implement JSON file handlers
9. Write test for directory listing (`ls /_metrics/cache/`)
10. Implement `ReadDir()` support for metrics paths

### Phase 4: Integration & Instrumentation
1. Write test verifying cache operations update collector
2. Instrument existing cache code in `ragfs.go`
3. Write test verifying I/O operations update collector
4. Instrument file read operations
5. Write test verifying errors are tracked
6. Instrument error paths
7. Write test for system metrics accuracy
8. Implement system metrics collection in FUSE layer

### Phase 5: End-to-End Verification
1. Integration test: mount filesystem, verify `/_metrics/` tree exists
2. Integration test: perform operations, verify metrics update
3. Integration test: parse JSON blocks from summary files
4. Integration test: verify all required paths exist
5. **MANDATORY:** Manual verification with JSON and SQLite examples using foreground shell commands

---

## File Structure

**New files:**
- `metrics.go` - Collector implementation
- `metrics_fs.go` - Filesystem handler for `/_metrics/` paths
- `metrics_test.go` - Unit tests for collector
- `metrics_fs_test.go` - Unit tests for filesystem handler

**Modified files:**
- `ragfs.go` - Add collector, instrument cache/IO/errors
- `fuse.go` - Add system metrics collection

---

## Error Sanitization Patterns

Regex patterns to redact from error messages:
- API keys: `(api[_-]?key|apikey)[\s:=]+[^\s]+`
- Tokens: `(token|bearer)[\s:=]+[^\s]+`
- Passwords: `(password|passwd|pwd)[\s:=]+[^\s]+`
- Replace matches with `[REDACTED]`

---

## Verification Checklist (Phase 5)

**JSON Example:**
```bash
go build -o json-mount examples/json/main.go
./json-mount -mount /tmp/json-test -config config.json &
sleep 2
ls /tmp/json-test/
ls /tmp/json-test/_metrics/
cat /tmp/json-test/_metrics/summary.md
cat /tmp/json-test/_metrics/version.txt
ls /tmp/json-test/_metrics/cache/
cat /tmp/json-test/_metrics/cache/cache_hit_count
kill %1
```

**SQLite Example:**
```bash
go build -o sqlite-mount examples/sqlite/main.go
./sqlite-mount -mount /tmp/sqlite-test -db examples/sqlite/example.db &
sleep 2
ls /tmp/sqlite-test/
ls /tmp/sqlite-test/_metrics/
cat /tmp/sqlite-test/_metrics/summary.md
cat /tmp/sqlite-test/_metrics/io/bytes_read_total
cat /tmp/sqlite-test/_metrics/errors/errors_total
kill %1
```

---

## Success Criteria

- All unit tests pass
- All integration tests pass
- `/_metrics/` tree accessible on both JSON and SQLite examples
- All required paths exist and return valid content
- Markdown summaries parse correctly
- JSON blocks are valid JSON
- Metrics update in real-time as operations occur
