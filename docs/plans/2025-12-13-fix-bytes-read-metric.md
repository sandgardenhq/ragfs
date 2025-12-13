# Fix bytes_read Metric - Implementation Plan

**Issue:** #8 - Metrics: bytes_read always shows 0

**Goal:** Fix the bytes_read metric so it correctly tracks bytes read from files.

---

## Problem Analysis

The `bytes_read` metric always shows 0 because `RecordRead()` is never called in the production code path.

### Current Code Flow

1. `FS.Open()` returns a `file` struct containing a `bytes.Reader`
2. `file.Read()` calls `f.reader.Read(p)` and returns the count
3. **Missing:** No call to `collector.RecordRead()` anywhere in this flow

### Root Cause

The `file` struct only contains `name` and `reader` - it has no reference to the collector:

```go
type file struct {
    name   string
    reader *bytes.Reader
}
```

When `file.Read()` is called, it cannot record metrics because it has no access to the collector.

---

## Solution

Add a `collector` reference to the `file` struct and call `RecordRead()` in the `Read()` method.

---

## Task 1: Add collector to file struct

**File:** `ragfs.go`

### Step 1: Modify file struct

```go
type file struct {
    name      string
    reader    *bytes.Reader
    collector *Collector
    startTime time.Time
}
```

### Step 2: Update Open() to pass collector

In the `Open` method, when creating the file struct, pass the collector:

```go
return &file{
    name:      entry.Name(),
    reader:    bytes.NewReader(content),
    collector: f.collector,
    startTime: time.Now(),
}, nil
```

### Step 3: Update Read() to record metrics

```go
func (f *file) Read(p []byte) (int, error) {
    n, err := f.reader.Read(p)
    if n > 0 && f.collector != nil {
        f.collector.RecordRead(int64(n), time.Since(f.startTime))
    }
    return n, err
}
```

---

## Task 2: Write tests

**File:** `ragfs_test.go` or `write_test.go`

### Test: Verify bytes_read is recorded

```go
func TestBytesReadMetric(t *testing.T) {
    fsys := ragfs.New()

    // Map a handler with known content
    fsys.Map("/test", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
        return []fs.DirEntry{ragfs.NewFileEntry("test.txt", []byte("hello world"))}, nil
    }))

    // Read the file
    f, err := fsys.Open("/test")
    if err != nil {
        t.Fatal(err)
    }

    content, err := io.ReadAll(f)
    f.Close()

    if err != nil {
        t.Fatal(err)
    }

    // Verify content was read
    if string(content) != "hello world" {
        t.Errorf("Expected 'hello world', got %q", content)
    }

    // Check metrics - bytes_read should be 11
    snapshot := fsys.Collector().Snapshot()
    if snapshot.BytesRead != 11 {
        t.Errorf("Expected BytesRead=11, got %d", snapshot.BytesRead)
    }
}
```

---

## Task 3: Verify with FUSE mount

1. Build writable example
2. Mount and create a file with known content
3. Read the file with `cat`
4. Check `/_metrics/io/bytes_read`

---

## Task 4: Update documentation if needed

No documentation changes expected - this is a bug fix.

---

## Verification Checklist

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] BytesRead metric shows correct value after reading files
- [ ] FUSE mount verification shows bytes_read updating
