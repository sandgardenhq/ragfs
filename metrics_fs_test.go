package ragfs

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"
)

// TestMetricsFS_Integration tests the full /_metrics/ filesystem tree.
func TestMetricsFS_Integration(t *testing.T) {
	fsys := New()

	// Add a dummy handler for testing
	fsys.Map("/test/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			NewFileEntry("test.txt", []byte("test data")),
		}, nil
	})

	// Test root directory listing
	t.Run("root directory", func(t *testing.T) {
		entries, err := fsys.ReadDir("/_metrics")
		if err != nil {
			t.Fatalf("ReadDir(/_metrics) error: %v", err)
		}

		expectedEntries := []string{".", "..", "summary.md", "version.txt", "cache", "io", "system", "errors"}
		if len(entries) != len(expectedEntries) {
			t.Errorf("ReadDir(/_metrics) returned %d entries, want %d", len(entries), len(expectedEntries))
		}

		for _, name := range expectedEntries {
			found := false
			for _, entry := range entries {
				if entry.Name() == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected entry %s not found in /_metrics", name)
			}
		}
	})

	// Test version file
	t.Run("version.txt", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/version.txt")
		if err != nil {
			t.Fatalf("Open(/_metrics/version.txt) error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		if string(content) != "v1\n" {
			t.Errorf("version.txt = %q, want %q", string(content), "v1\n")
		}
	})

	// Test summary.md
	t.Run("summary.md", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/summary.md")
		if err != nil {
			t.Fatalf("Open(/_metrics/summary.md) error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		summary := string(content)
		if !strings.Contains(summary, "# Metrics Summary") {
			t.Error("summary.md missing title")
		}
		if !strings.Contains(summary, "## Quick Stats") {
			t.Error("summary.md missing Quick Stats section")
		}
		if !strings.Contains(summary, "| Metric | Value |") {
			t.Error("summary.md missing table")
		}
	})

	// Test cache directory
	t.Run("cache directory", func(t *testing.T) {
		entries, err := fsys.ReadDir("/_metrics/cache")
		if err != nil {
			t.Fatalf("ReadDir(/_metrics/cache) error: %v", err)
		}

		expectedFiles := []string{"summary.md", "cache_hit_count", "cache_miss_count", "cache_hit_rate", "cache_entries", "cache_evictions"}
		for _, name := range expectedFiles {
			found := false
			for _, entry := range entries {
				if entry.Name() == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected file %s not found in /_metrics/cache", name)
			}
		}
	})

	// Test cache metrics (with cache disabled)
	t.Run("cache metrics disabled", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/cache/summary.md")
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		if !strings.Contains(string(content), "Cache is **disabled**") {
			t.Error("cache summary should indicate cache is disabled")
		}
	})

	// Test I/O directory
	t.Run("io directory", func(t *testing.T) {
		entries, err := fsys.ReadDir("/_metrics/io")
		if err != nil {
			t.Fatalf("ReadDir(/_metrics/io) error: %v", err)
		}

		expectedFiles := []string{"summary.md", "bytes_read_total", "bytes_written_total", "read_ops_total", "write_ops_total", "avg_read_latency_ms", "avg_write_latency_ms", "last_read_time", "last_write_time"}
		for _, name := range expectedFiles {
			found := false
			for _, entry := range entries {
				if entry.Name() == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected file %s not found in /_metrics/io", name)
			}
		}
	})

	// Test I/O metrics
	t.Run("io metrics", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/io/read_ops_total")
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		// Should be "0\n" since no reads yet
		if string(content) != "0\n" {
			t.Errorf("read_ops_total = %q, want %q", string(content), "0\n")
		}
	})

	// Test system directory
	t.Run("system directory", func(t *testing.T) {
		entries, err := fsys.ReadDir("/_metrics/system")
		if err != nil {
			t.Fatalf("ReadDir(/_metrics/system) error: %v", err)
		}

		expectedFiles := []string{"summary.md", "uptime_seconds", "mounted_at", "process_pid", "memory_rss_bytes", "goroutines"}
		for _, name := range expectedFiles {
			found := false
			for _, entry := range entries {
				if entry.Name() == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected file %s not found in /_metrics/system", name)
			}
		}
	})

	// Test system metrics
	t.Run("system metrics", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/system/process_pid")
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		// Should be a number
		pid := strings.TrimSpace(string(content))
		if pid == "" || pid == "0" {
			t.Errorf("process_pid should not be empty or 0, got %q", pid)
		}
	})

	// Test errors directory
	t.Run("errors directory", func(t *testing.T) {
		entries, err := fsys.ReadDir("/_metrics/errors")
		if err != nil {
			t.Fatalf("ReadDir(/_metrics/errors) error: %v", err)
		}

		expectedFiles := []string{"summary.md", "errors_total", "last_error_time", "last_error_message", "errors_by_type.json", "recent_errors.json"}
		for _, name := range expectedFiles {
			found := false
			for _, entry := range entries {
				if entry.Name() == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected file %s not found in /_metrics/errors", name)
			}
		}
	})

	// Test errors metrics (no errors yet)
	t.Run("errors metrics empty", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/errors/errors_total")
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		if string(content) != "0\n" {
			t.Errorf("errors_total = %q, want %q", string(content), "0\n")
		}
	})

	// Test nonexistent path
	t.Run("nonexistent path", func(t *testing.T) {
		_, err := fsys.Open("/_metrics/nonexistent")
		if err == nil {
			t.Error("Open(/_metrics/nonexistent) should return error")
		}
	})
}

// TestMetricsFS_WithCache tests metrics with caching enabled.
func TestMetricsFS_WithCache(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        30,
	})

	// Test cache metrics show enabled
	t.Run("cache enabled", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/cache/summary.md")
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		summary := string(content)
		if strings.Contains(summary, "Cache is **disabled**") {
			t.Error("cache summary should not indicate disabled when cache is enabled")
		}
		if !strings.Contains(summary, "| Hit Count |") {
			t.Error("cache summary missing Hit Count")
		}
	})

	// Test individual cache metrics
	t.Run("cache hit rate", func(t *testing.T) {
		f, err := fsys.Open("/_metrics/cache/cache_hit_rate")
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}

		// Should be "0.0000\n" initially
		rate := string(content)
		if !strings.HasPrefix(rate, "0.") {
			t.Errorf("cache_hit_rate = %q, should start with 0.", rate)
		}
	})
}

// TestMetricsFS_SummaryFormat tests that all summaries are valid Markdown.
func TestMetricsFS_SummaryFormat(t *testing.T) {
	fsys := New()

	summaries := []string{
		"/_metrics/summary.md",
		"/_metrics/cache/summary.md",
		"/_metrics/io/summary.md",
		"/_metrics/system/summary.md",
		"/_metrics/errors/summary.md",
	}

	for _, path := range summaries {
		t.Run(path, func(t *testing.T) {
			f, err := fsys.Open(path)
			if err != nil {
				t.Fatalf("Open(%s) error: %v", path, err)
			}
			defer f.Close()

			content, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("ReadAll error: %v", err)
			}

			summary := string(content)

			// Check for Markdown heading
			if !strings.HasPrefix(summary, "#") {
				t.Errorf("%s should start with Markdown heading", path)
			}

			// Check for Generated timestamp
			if !strings.Contains(summary, "**Generated:**") {
				t.Errorf("%s missing Generated timestamp", path)
			}

			// Check timestamp is RFC3339 format
			if strings.Contains(summary, "**Generated:**") && !strings.Contains(summary, "T") {
				t.Errorf("%s timestamp should be RFC3339 format with T separator", path)
			}
		})
	}
}

// TestMetricsFS_JSONFiles tests that JSON files are valid.
func TestMetricsFS_JSONFiles(t *testing.T) {
	fsys := New()

	jsonFiles := []string{
		"/_metrics/errors/errors_by_type.json",
		"/_metrics/errors/recent_errors.json",
	}

	for _, path := range jsonFiles {
		t.Run(path, func(t *testing.T) {
			f, err := fsys.Open(path)
			if err != nil {
				t.Fatalf("Open(%s) error: %v", path, err)
			}
			defer f.Close()

			content, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("ReadAll error: %v", err)
			}

			// Check it's valid JSON (starts with { or [)
			trimmed := strings.TrimSpace(string(content))
			if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
				t.Errorf("%s should be valid JSON, got: %s", path, trimmed)
			}

			// Check it ends with newline
			if !strings.HasSuffix(string(content), "\n") {
				t.Errorf("%s should end with newline", path)
			}
		})
	}
}
