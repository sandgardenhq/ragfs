package ragfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
	"time"
)

// registerMetricsHandler registers the /_metrics/** handler with the filesystem.
// This should be called during FS initialization.
func (f *FS) registerMetricsHandler() {
	if f.metrics == nil {
		return
	}

	// Register the /_metrics pattern
	f.Map("/_metrics/**", f.metricsHandler)
}

// metricsHandler handles all requests to /_metrics/** paths.
func (f *FS) metricsHandler(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	if f.metrics == nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}

	// Remove /_metrics prefix
	subpath := strings.TrimPrefix(path, "/_metrics")
	if subpath == "" || subpath == "/" {
		// Root /_metrics/ directory
		return f.metricsRootDir(), nil
	}

	// Route to appropriate handler based on subpath
	switch {
	case subpath == "/summary.md":
		return f.metricsSummaryFile(), nil
	case subpath == "/version.txt":
		return f.metricsVersionFile(), nil

	// Cache metrics
	case strings.HasPrefix(subpath, "/cache"):
		return f.metricsCacheHandler(subpath)

	// I/O metrics
	case strings.HasPrefix(subpath, "/io"):
		return f.metricsIOHandler(subpath)

	// System metrics
	case strings.HasPrefix(subpath, "/system"):
		return f.metricsSystemHandler(subpath)

	// Error metrics
	case strings.HasPrefix(subpath, "/errors"):
		return f.metricsErrorsHandler(subpath)

	default:
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
}

// metricsRootDir returns the root /_metrics/ directory listing.
func (f *FS) metricsRootDir() []fs.DirEntry {
	return []fs.DirEntry{
		NewDirEntry(".", true),
		NewDirEntry("..", true),
		NewFileEntry("summary.md", []byte("# Metrics Summary\n")),
		NewFileEntry("version.txt", []byte("v1\n")),
		NewDirEntry("cache", true),
		NewDirEntry("io", true),
		NewDirEntry("system", true),
		NewDirEntry("errors", true),
	}
}

// metricsVersionFile returns the version.txt file content.
func (f *FS) metricsVersionFile() []fs.DirEntry {
	return []fs.DirEntry{
		NewFileEntry("version.txt", []byte("v1\n")),
	}
}

// metricsSummaryFile returns the top-level summary.md file.
func (f *FS) metricsSummaryFile() []fs.DirEntry {
	content := f.generateTopLevelSummary()
	return []fs.DirEntry{
		NewFileEntry("summary.md", []byte(content)),
	}
}

// metricsCacheHandler handles /cache/** paths.
func (f *FS) metricsCacheHandler(subpath string) ([]fs.DirEntry, error) {
	subpath = strings.TrimPrefix(subpath, "/cache")

	if subpath == "" || subpath == "/" {
		// /cache/ directory listing
		return f.metricsCacheDir(), nil
	}

	// Individual cache metric files
	switch subpath {
	case "/summary.md":
		content := f.generateCacheSummary()
		return []fs.DirEntry{NewFileEntry("summary.md", []byte(content))}, nil
	case "/cache_hit_count":
		val := int64(0)
		if f.metrics.cacheStats != nil {
			val = f.metrics.cacheStats.Hits.Load()
		}
		return []fs.DirEntry{NewFileEntry("cache_hit_count", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/cache_miss_count":
		val := int64(0)
		if f.metrics.cacheStats != nil {
			val = f.metrics.cacheStats.Misses.Load()
		}
		return []fs.DirEntry{NewFileEntry("cache_miss_count", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/cache_hit_rate":
		rate := 0.0
		if f.metrics.cacheStats != nil {
			hits := f.metrics.cacheStats.Hits.Load()
			misses := f.metrics.cacheStats.Misses.Load()
			total := hits + misses
			if total > 0 {
				rate = float64(hits) / float64(total)
			}
		}
		return []fs.DirEntry{NewFileEntry("cache_hit_rate", []byte(fmt.Sprintf("%.4f\n", rate)))}, nil
	case "/cache_entries":
		val := int64(0)
		if f.metrics.cacheStats != nil {
			val = f.metrics.cacheStats.Entries.Load()
		}
		return []fs.DirEntry{NewFileEntry("cache_entries", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/cache_evictions":
		val := int64(0)
		if f.metrics.cacheStats != nil {
			val = f.metrics.cacheStats.Evictions.Load()
		}
		return []fs.DirEntry{NewFileEntry("cache_evictions", []byte(fmt.Sprintf("%d\n", val)))}, nil
	default:
		return nil, &fs.PathError{Op: "open", Path: "/_metrics/cache" + subpath, Err: fs.ErrNotExist}
	}
}

// metricsCacheDir returns the /cache/ directory listing.
func (f *FS) metricsCacheDir() []fs.DirEntry {
	return []fs.DirEntry{
		NewDirEntry(".", true),
		NewDirEntry("..", true),
		NewFileEntry("summary.md", []byte("# Cache Metrics\n")),
		NewFileEntry("cache_hit_count", []byte("0\n")),
		NewFileEntry("cache_miss_count", []byte("0\n")),
		NewFileEntry("cache_hit_rate", []byte("0.0000\n")),
		NewFileEntry("cache_entries", []byte("0\n")),
		NewFileEntry("cache_evictions", []byte("0\n")),
	}
}

// metricsIOHandler handles /io/** paths.
func (f *FS) metricsIOHandler(subpath string) ([]fs.DirEntry, error) {
	subpath = strings.TrimPrefix(subpath, "/io")

	if subpath == "" || subpath == "/" {
		// /io/ directory listing
		return f.metricsIODir(), nil
	}

	// Individual I/O metric files
	switch subpath {
	case "/summary.md":
		content := f.generateIOSummary()
		return []fs.DirEntry{NewFileEntry("summary.md", []byte(content))}, nil
	case "/bytes_read_total":
		val := f.metrics.bytesRead.Load()
		return []fs.DirEntry{NewFileEntry("bytes_read_total", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/bytes_written_total":
		val := f.metrics.bytesWritten.Load()
		return []fs.DirEntry{NewFileEntry("bytes_written_total", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/read_ops_total":
		val := f.metrics.readOps.Load()
		return []fs.DirEntry{NewFileEntry("read_ops_total", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/write_ops_total":
		val := f.metrics.writeOps.Load()
		return []fs.DirEntry{NewFileEntry("write_ops_total", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/avg_read_latency_ms":
		val := f.metrics.AvgReadLatency()
		return []fs.DirEntry{NewFileEntry("avg_read_latency_ms", []byte(fmt.Sprintf("%.2f\n", val)))}, nil
	case "/avg_write_latency_ms":
		val := f.metrics.AvgWriteLatency()
		return []fs.DirEntry{NewFileEntry("avg_write_latency_ms", []byte(fmt.Sprintf("%.2f\n", val)))}, nil
	case "/last_read_time":
		t := f.metrics.GetLastReadTime()
		return []fs.DirEntry{NewFileEntry("last_read_time", []byte(formatTimestamp(t) + "\n"))}, nil
	case "/last_write_time":
		t := f.metrics.GetLastWriteTime()
		return []fs.DirEntry{NewFileEntry("last_write_time", []byte(formatTimestamp(t) + "\n"))}, nil
	default:
		return nil, &fs.PathError{Op: "open", Path: "/_metrics/io" + subpath, Err: fs.ErrNotExist}
	}
}

// metricsIODir returns the /io/ directory listing.
func (f *FS) metricsIODir() []fs.DirEntry {
	return []fs.DirEntry{
		NewDirEntry(".", true),
		NewDirEntry("..", true),
		NewFileEntry("summary.md", []byte("# I/O Metrics\n")),
		NewFileEntry("bytes_read_total", []byte("0\n")),
		NewFileEntry("bytes_written_total", []byte("0\n")),
		NewFileEntry("read_ops_total", []byte("0\n")),
		NewFileEntry("write_ops_total", []byte("0\n")),
		NewFileEntry("avg_read_latency_ms", []byte("0.00\n")),
		NewFileEntry("avg_write_latency_ms", []byte("0.00\n")),
		NewFileEntry("last_read_time", []byte("never\n")),
		NewFileEntry("last_write_time", []byte("never\n")),
	}
}

// metricsSystemHandler handles /system/** paths.
func (f *FS) metricsSystemHandler(subpath string) ([]fs.DirEntry, error) {
	subpath = strings.TrimPrefix(subpath, "/system")

	if subpath == "" || subpath == "/" {
		// /system/ directory listing
		return f.metricsSystemDir(), nil
	}

	// Individual system metric files
	switch subpath {
	case "/summary.md":
		content := f.generateSystemSummary()
		return []fs.DirEntry{NewFileEntry("summary.md", []byte(content))}, nil
	case "/uptime_seconds":
		val := int64(f.metrics.Uptime().Seconds())
		return []fs.DirEntry{NewFileEntry("uptime_seconds", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/mounted_at":
		t := f.metrics.mountedAt
		return []fs.DirEntry{NewFileEntry("mounted_at", []byte(formatTimestamp(t) + "\n"))}, nil
	case "/process_pid":
		val := f.metrics.pid
		return []fs.DirEntry{NewFileEntry("process_pid", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/memory_rss_bytes":
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return []fs.DirEntry{NewFileEntry("memory_rss_bytes", []byte(fmt.Sprintf("%d\n", m.Alloc)))}, nil
	case "/goroutines":
		val := runtime.NumGoroutine()
		return []fs.DirEntry{NewFileEntry("goroutines", []byte(fmt.Sprintf("%d\n", val)))}, nil
	default:
		return nil, &fs.PathError{Op: "open", Path: "/_metrics/system" + subpath, Err: fs.ErrNotExist}
	}
}

// metricsSystemDir returns the /system/ directory listing.
func (f *FS) metricsSystemDir() []fs.DirEntry {
	return []fs.DirEntry{
		NewDirEntry(".", true),
		NewDirEntry("..", true),
		NewFileEntry("summary.md", []byte("# System Metrics\n")),
		NewFileEntry("uptime_seconds", []byte("0\n")),
		NewFileEntry("mounted_at", []byte("never\n")),
		NewFileEntry("process_pid", []byte("0\n")),
		NewFileEntry("memory_rss_bytes", []byte("0\n")),
		NewFileEntry("goroutines", []byte("0\n")),
	}
}

// metricsErrorsHandler handles /errors/** paths.
func (f *FS) metricsErrorsHandler(subpath string) ([]fs.DirEntry, error) {
	subpath = strings.TrimPrefix(subpath, "/errors")

	if subpath == "" || subpath == "/" {
		// /errors/ directory listing
		return f.metricsErrorsDir(), nil
	}

	// Individual error metric files
	switch subpath {
	case "/summary.md":
		content := f.generateErrorsSummary()
		return []fs.DirEntry{NewFileEntry("summary.md", []byte(content))}, nil
	case "/errors_total":
		val := f.metrics.errorsTotal.Load()
		return []fs.DirEntry{NewFileEntry("errors_total", []byte(fmt.Sprintf("%d\n", val)))}, nil
	case "/last_error_time":
		lastErr := f.metrics.GetLastError()
		if lastErr == nil {
			return []fs.DirEntry{NewFileEntry("last_error_time", []byte("never\n"))}, nil
		}
		return []fs.DirEntry{NewFileEntry("last_error_time", []byte(formatTimestamp(lastErr.Timestamp) + "\n"))}, nil
	case "/last_error_message":
		lastErr := f.metrics.GetLastError()
		if lastErr == nil {
			return []fs.DirEntry{NewFileEntry("last_error_message", []byte("none\n"))}, nil
		}
		return []fs.DirEntry{NewFileEntry("last_error_message", []byte(lastErr.Message + "\n"))}, nil
	case "/errors_by_type.json":
		errorsByType := f.metrics.GetErrorsByType()
		data, _ := json.MarshalIndent(errorsByType, "", "  ")
		return []fs.DirEntry{NewFileEntry("errors_by_type.json", append(data, '\n'))}, nil
	case "/recent_errors.json":
		recentErrors := f.metrics.GetRecentErrors()
		data, _ := json.MarshalIndent(recentErrors, "", "  ")
		return []fs.DirEntry{NewFileEntry("recent_errors.json", append(data, '\n'))}, nil
	default:
		return nil, &fs.PathError{Op: "open", Path: "/_metrics/errors" + subpath, Err: fs.ErrNotExist}
	}
}

// metricsErrorsDir returns the /errors/ directory listing.
func (f *FS) metricsErrorsDir() []fs.DirEntry {
	return []fs.DirEntry{
		NewDirEntry(".", true),
		NewDirEntry("..", true),
		NewFileEntry("summary.md", []byte("# Error Metrics\n")),
		NewFileEntry("errors_total", []byte("0\n")),
		NewFileEntry("last_error_time", []byte("never\n")),
		NewFileEntry("last_error_message", []byte("none\n")),
		NewFileEntry("errors_by_type.json", []byte("{}\n")),
		NewFileEntry("recent_errors.json", []byte("[]\n")),
	}
}

// generateTopLevelSummary generates the top-level summary.md content.
func (f *FS) generateTopLevelSummary() string {
	now := time.Now().Format(time.RFC3339)
	uptime := formatDuration(f.metrics.Uptime())

	// Get key stats
	readOps := f.metrics.readOps.Load()
	writeOps := f.metrics.writeOps.Load()
	errorsTotal := f.metrics.errorsTotal.Load()

	cacheHits := int64(0)
	cacheMisses := int64(0)
	if f.metrics.cacheStats != nil {
		cacheHits = f.metrics.cacheStats.Hits.Load()
		cacheMisses = f.metrics.cacheStats.Misses.Load()
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	summary := fmt.Sprintf(`# Metrics Summary

**Generated:** %s
**Uptime:** %s

## Quick Stats

| Metric | Value |
|--------|-------|
| Read Operations | %d |
| Write Operations | %d |
| Cache Hits | %d |
| Cache Misses | %d |
| Total Errors | %d |
| Memory (RSS) | %d bytes |
| Goroutines | %d |

## Details

- **Cache:** See [/_metrics/cache/summary.md](/_metrics/cache/summary.md)
- **I/O:** See [/_metrics/io/summary.md](/_metrics/io/summary.md)
- **System:** See [/_metrics/system/summary.md](/_metrics/system/summary.md)
- **Errors:** See [/_metrics/errors/summary.md](/_metrics/errors/summary.md)

`, now, uptime, readOps, writeOps, cacheHits, cacheMisses, errorsTotal, m.Alloc, runtime.NumGoroutine())

	return summary
}

// generateCacheSummary generates the cache summary.md content.
func (f *FS) generateCacheSummary() string {
	now := time.Now().Format(time.RFC3339)

	if f.metrics.cacheStats == nil {
		return fmt.Sprintf(`# Cache Metrics

**Generated:** %s

Cache is **disabled** for this filesystem.

`, now)
	}

	hits := f.metrics.cacheStats.Hits.Load()
	misses := f.metrics.cacheStats.Misses.Load()
	entries := f.metrics.cacheStats.Entries.Load()
	evictions := f.metrics.cacheStats.Evictions.Load()

	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	return fmt.Sprintf(`# Cache Metrics

**Generated:** %s

| Metric | Value |
|--------|-------|
| Hit Count | %d |
| Miss Count | %d |
| Hit Rate | %.2f%% |
| Entries | %d |
| Evictions | %d |

`, now, hits, misses, hitRate, entries, evictions)
}

// generateIOSummary generates the I/O summary.md content.
func (f *FS) generateIOSummary() string {
	now := time.Now().Format(time.RFC3339)

	bytesRead := f.metrics.bytesRead.Load()
	bytesWritten := f.metrics.bytesWritten.Load()
	readOps := f.metrics.readOps.Load()
	writeOps := f.metrics.writeOps.Load()
	avgReadLatency := f.metrics.AvgReadLatency()
	avgWriteLatency := f.metrics.AvgWriteLatency()
	lastReadTime := f.metrics.GetLastReadTime()
	lastWriteTime := f.metrics.GetLastWriteTime()

	return fmt.Sprintf(`# I/O Metrics

**Generated:** %s

| Metric | Value |
|--------|-------|
| Bytes Read | %d |
| Bytes Written | %d |
| Read Operations | %d |
| Write Operations | %d |
| Avg Read Latency | %.2f ms |
| Avg Write Latency | %.2f ms |
| Last Read | %s |
| Last Write | %s |

`, now, bytesRead, bytesWritten, readOps, writeOps, avgReadLatency, avgWriteLatency,
		formatTimestamp(lastReadTime), formatTimestamp(lastWriteTime))
}

// generateSystemSummary generates the system summary.md content.
func (f *FS) generateSystemSummary() string {
	now := time.Now().Format(time.RFC3339)
	uptime := formatDuration(f.metrics.Uptime())
	mountedAt := formatTimestamp(f.metrics.mountedAt)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return fmt.Sprintf(`# System Metrics

**Generated:** %s

| Metric | Value |
|--------|-------|
| Uptime | %s |
| Mounted At | %s |
| Process PID | %d |
| Memory (Alloc) | %d bytes |
| Goroutines | %d |

`, now, uptime, mountedAt, f.metrics.pid, m.Alloc, runtime.NumGoroutine())
}

// generateErrorsSummary generates the errors summary.md content.
func (f *FS) generateErrorsSummary() string {
	now := time.Now().Format(time.RFC3339)

	errorsTotal := f.metrics.errorsTotal.Load()
	lastErr := f.metrics.GetLastError()
	errorsByType := f.metrics.GetErrorsByType()

	lastErrTime := "never"
	lastErrMsg := "none"
	if lastErr != nil {
		lastErrTime = formatTimestamp(lastErr.Timestamp)
		lastErrMsg = lastErr.Message
	}

	summary := fmt.Sprintf(`# Error Metrics

**Generated:** %s

| Metric | Value |
|--------|-------|
| Total Errors | %d |
| Last Error Time | %s |
| Last Error | %s |

`, now, errorsTotal, lastErrTime, lastErrMsg)

	if len(errorsByType) > 0 {
		summary += "\n## Errors by Type\n\n"
		summary += "| Type | Count |\n"
		summary += "|------|-------|\n"
		for kind, count := range errorsByType {
			summary += fmt.Sprintf("| %s | %d |\n", kind, count)
		}
		summary += "\n"
	}

	return summary
}
