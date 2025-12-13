package ragfs_test

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/brittcrawford/ragfs"
)

// Test: FS integrates with MetricsFS - verify /_metrics/ path exists
func TestIntegration_MetricsAvailable(t *testing.T) {
	fsys := ragfs.New()

	// Add a simple handler
	fsys.Map("/test", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{&testFileEntry{name: "test.txt", content: []byte("hello")}}, nil
	}))

	// Access /_metrics/version.txt
	file, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("Failed to open /_metrics/version.txt: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read /_metrics/version.txt: %v", err)
	}

	if string(content) != "v1" {
		t.Errorf("Expected 'v1', got '%s'", string(content))
	}
}

// Test: Cache operations update metrics
func TestIntegration_CacheMetricsTracking(t *testing.T) {
	fsys := ragfs.New()
	fsys.EnableCache(ragfs.CacheConfig{
		MaxEntries: 10,
		TTL:        time.Minute,
	})

	// Add a handler
	callCount := 0
	fsys.Map("/test", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{&testFileEntry{name: "test.txt", content: []byte("hello")}}, nil
	}))

	// First access - cache miss
	file1, _ := fsys.Open("/test")
	io.ReadAll(file1)
	file1.Close()

	// Second access - cache hit
	file2, _ := fsys.Open("/test")
	io.ReadAll(file2)
	file2.Close()

	// Read cache hit count from metrics
	metricsFile, err := fsys.Open("/_metrics/cache/cache_hit_count")
	if err != nil {
		t.Fatalf("Failed to open cache_hit_count: %v", err)
	}
	defer metricsFile.Close()

	content, _ := io.ReadAll(metricsFile)
	hitCount := strings.TrimSpace(string(content))

	// Should have at least 1 cache hit
	if hitCount == "0" {
		t.Error("Expected cache hits > 0 after cached access")
	}
}

// Test: /_metrics/summary.md contains valid markdown and JSON
func TestIntegration_SummaryGeneration(t *testing.T) {
	fsys := ragfs.New()

	// Add a handler and trigger some activity
	fsys.Map("/test", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{&testFileEntry{name: "test.txt", content: []byte("hello")}}, nil
	}))

	// Access the test path to generate some metrics
	file, _ := fsys.Open("/test")
	io.ReadAll(file)
	file.Close()

	// Read summary
	summaryFile, err := fsys.Open("/_metrics/summary.md")
	if err != nil {
		t.Fatalf("Failed to open summary.md: %v", err)
	}
	defer summaryFile.Close()

	content, _ := io.ReadAll(summaryFile)
	summary := string(content)

	// Verify markdown structure
	if !strings.Contains(summary, "# ragfs Metrics Summary") {
		t.Error("Summary should contain main header")
	}

	if !strings.Contains(summary, "| Metric | Value |") {
		t.Error("Summary should contain table header")
	}

	// Verify JSON block
	if !strings.Contains(summary, "```json") {
		t.Error("Summary should contain JSON code block")
	}
}

// Test: ReadDir works on /_metrics/ directory
func TestIntegration_MetricsDirectoryListing(t *testing.T) {
	fsys := ragfs.New()

	// List /_metrics/ directory
	entries, err := fsys.ReadDir("/_metrics")
	if err != nil {
		t.Fatalf("Failed to read /_metrics directory: %v", err)
	}

	// Should contain standard entries
	expectedNames := []string{"version.txt", "summary.md", "cache", "io", "system", "errors"}
	foundNames := make(map[string]bool)
	for _, entry := range entries {
		foundNames[entry.Name()] = true
	}

	for _, expected := range expectedNames {
		if !foundNames[expected] {
			t.Errorf("Expected to find '%s' in /_metrics directory", expected)
		}
	}
}
