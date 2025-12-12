package ragfs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// MetricsFS implements a virtual read-only filesystem at /_metrics/ that exposes
// runtime metrics through a hierarchical file structure.
type MetricsFS struct {
	collector *Collector
}

// NewMetricsFS creates a new metrics filesystem backed by the given collector.
func NewMetricsFS(collector *Collector) *MetricsFS {
	return &MetricsFS{
		collector: collector,
	}
}

// Open opens the named file from the metrics filesystem.
// It implements the fs.FS interface.
func (mfs *MetricsFS) Open(name string) (fs.File, error) {
	// Normalize path
	name = path.Clean(name)
	if !strings.HasPrefix(name, "/_metrics") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	snapshot := mfs.collector.Snapshot()

	// Route to appropriate handler
	switch name {
	case "/_metrics":
		return &metricsDir{name: "_metrics", isRoot: true}, nil

	case "/_metrics/version.txt":
		return &metricsFile{
			name:    "version.txt",
			content: []byte("v1"),
		}, nil

	case "/_metrics/summary.md":
		content := GenerateSummaryMarkdown(snapshot)
		return &metricsFile{
			name:    "summary.md",
			content: []byte(content),
		}, nil

	// Cache group
	case "/_metrics/cache":
		return &metricsDir{name: "cache", isRoot: false}, nil

	case "/_metrics/cache/summary.md":
		content := GenerateCacheSummaryMarkdown(snapshot)
		return &metricsFile{
			name:    "summary.md",
			content: []byte(content),
		}, nil

	case "/_metrics/cache/cache_hit_count":
		return &metricsFile{
			name:    "cache_hit_count",
			content: []byte(fmt.Sprintf("%d\n", snapshot.CacheHits)),
		}, nil

	case "/_metrics/cache/cache_miss_count":
		return &metricsFile{
			name:    "cache_miss_count",
			content: []byte(fmt.Sprintf("%d\n", snapshot.CacheMisses)),
		}, nil

	case "/_metrics/cache/cache_hit_rate":
		return &metricsFile{
			name:    "cache_hit_rate",
			content: []byte(fmt.Sprintf("%.3f\n", snapshot.CacheHitRate)),
		}, nil

	case "/_metrics/cache/cache_entries":
		return &metricsFile{
			name:    "cache_entries",
			content: []byte(fmt.Sprintf("%d\n", snapshot.CacheEntries)),
		}, nil

	case "/_metrics/cache/cache_evictions":
		return &metricsFile{
			name:    "cache_evictions",
			content: []byte(fmt.Sprintf("%d\n", snapshot.CacheEvictions)),
		}, nil

	// I/O group
	case "/_metrics/io":
		return &metricsDir{name: "io", isRoot: false}, nil

	case "/_metrics/io/summary.md":
		content := GenerateIOSummaryMarkdown(snapshot)
		return &metricsFile{
			name:    "summary.md",
			content: []byte(content),
		}, nil

	case "/_metrics/io/bytes_read":
		return &metricsFile{
			name:    "bytes_read",
			content: []byte(fmt.Sprintf("%d\n", snapshot.BytesRead)),
		}, nil

	case "/_metrics/io/bytes_written":
		return &metricsFile{
			name:    "bytes_written",
			content: []byte(fmt.Sprintf("%d\n", snapshot.BytesWritten)),
		}, nil

	case "/_metrics/io/read_ops":
		return &metricsFile{
			name:    "read_ops",
			content: []byte(fmt.Sprintf("%d\n", snapshot.ReadOps)),
		}, nil

	case "/_metrics/io/write_ops":
		return &metricsFile{
			name:    "write_ops",
			content: []byte(fmt.Sprintf("%d\n", snapshot.WriteOps)),
		}, nil

	// Error group
	case "/_metrics/errors":
		return &metricsDir{name: "errors", isRoot: false}, nil

	case "/_metrics/errors/summary.md":
		content := GenerateErrorSummaryMarkdown(snapshot)
		return &metricsFile{
			name:    "summary.md",
			content: []byte(content),
		}, nil

	case "/_metrics/errors/error_count":
		return &metricsFile{
			name:    "error_count",
			content: []byte(fmt.Sprintf("%d\n", snapshot.ErrorCount)),
		}, nil

	case "/_metrics/errors/errors_by_type.json":
		jsonBytes, _ := json.MarshalIndent(snapshot.ErrorsByType, "", "  ")
		return &metricsFile{
			name:    "errors_by_type.json",
			content: jsonBytes,
		}, nil

	// System group
	case "/_metrics/system":
		return &metricsDir{name: "system", isRoot: false}, nil

	default:
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
}

// metricsFile implements fs.File for a metrics file.
type metricsFile struct {
	name    string
	content []byte
	offset  int64
}

func (f *metricsFile) Stat() (fs.FileInfo, error) {
	return &metricsFileInfo{
		name: f.name,
		size: int64(len(f.content)),
	}, nil
}

func (f *metricsFile) Read(p []byte) (int, error) {
	if f.offset >= int64(len(f.content)) {
		return 0, io.EOF
	}
	n := copy(p, f.content[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *metricsFile) Close() error {
	return nil
}

// metricsDir implements fs.ReadDirFile for a metrics directory.
type metricsDir struct {
	name   string
	isRoot bool
	offset int
}

func (d *metricsDir) Stat() (fs.FileInfo, error) {
	return &metricsFileInfo{
		name:  d.name,
		isDir: true,
	}, nil
}

func (d *metricsDir) Read(p []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

func (d *metricsDir) Close() error {
	return nil
}

func (d *metricsDir) ReadDir(n int) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry

	if d.isRoot {
		// Root /_metrics directory
		entries = []fs.DirEntry{
			&metricsDirEntry{name: "version.txt", isDir: false},
			&metricsDirEntry{name: "summary.md", isDir: false},
			&metricsDirEntry{name: "cache", isDir: true},
			&metricsDirEntry{name: "io", isDir: true},
			&metricsDirEntry{name: "system", isDir: true},
			&metricsDirEntry{name: "errors", isDir: true},
		}
	} else if d.name == "cache" {
		// /_metrics/cache directory
		entries = []fs.DirEntry{
			&metricsDirEntry{name: "summary.md", isDir: false},
			&metricsDirEntry{name: "cache_hit_count", isDir: false},
			&metricsDirEntry{name: "cache_miss_count", isDir: false},
			&metricsDirEntry{name: "cache_hit_rate", isDir: false},
			&metricsDirEntry{name: "cache_entries", isDir: false},
			&metricsDirEntry{name: "cache_evictions", isDir: false},
		}
	} else if d.name == "io" {
		// /_metrics/io directory
		entries = []fs.DirEntry{
			&metricsDirEntry{name: "summary.md", isDir: false},
			&metricsDirEntry{name: "bytes_read", isDir: false},
			&metricsDirEntry{name: "bytes_written", isDir: false},
			&metricsDirEntry{name: "read_ops", isDir: false},
			&metricsDirEntry{name: "write_ops", isDir: false},
		}
	} else if d.name == "errors" {
		// /_metrics/errors directory
		entries = []fs.DirEntry{
			&metricsDirEntry{name: "summary.md", isDir: false},
			&metricsDirEntry{name: "error_count", isDir: false},
			&metricsDirEntry{name: "errors_by_type.json", isDir: false},
		}
	} else if d.name == "system" {
		// /_metrics/system directory (empty for now)
		entries = []fs.DirEntry{}
	}

	// Handle offset and limit
	if d.offset >= len(entries) {
		return nil, nil
	}

	if n <= 0 {
		// Return all remaining entries
		result := entries[d.offset:]
		d.offset = len(entries)
		return result, nil
	}

	// Return up to n entries
	end := d.offset + n
	if end > len(entries) {
		end = len(entries)
	}
	result := entries[d.offset:end]
	d.offset = end
	return result, nil
}

// metricsDirEntry implements fs.DirEntry for directory listings.
type metricsDirEntry struct {
	name  string
	isDir bool
}

func (e *metricsDirEntry) Name() string {
	return e.name
}

func (e *metricsDirEntry) IsDir() bool {
	return e.isDir
}

func (e *metricsDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

func (e *metricsDirEntry) Info() (fs.FileInfo, error) {
	return &metricsFileInfo{
		name:  e.name,
		isDir: e.isDir,
	}, nil
}

// metricsFileInfo implements fs.FileInfo for metrics files.
type metricsFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (fi *metricsFileInfo) Name() string {
	return fi.name
}

func (fi *metricsFileInfo) Size() int64 {
	return fi.size
}

func (fi *metricsFileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0555
	}
	return 0444
}

func (fi *metricsFileInfo) ModTime() time.Time {
	return time.Now()
}

func (fi *metricsFileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *metricsFileInfo) Sys() interface{} {
	return nil
}
