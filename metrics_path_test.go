package ragfs_test

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/brittcrawford/ragfs"
)

// Test that /_metrics routes match even with a wildcard /* handler
func TestMetricsRouteWithWildcardHandler(t *testing.T) {
	fsys := ragfs.New()

	// Register a wildcard handler AFTER metrics (simulating user code)
	wildcardCalled := false
	fsys.Map("/*", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		wildcardCalled = true
		return []fs.DirEntry{
			&testFileEntry{name: "user.txt", content: []byte("from wildcard")},
		}, nil
	}))

	// Test that /_metrics/version.txt is accessible
	file, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("/_metrics/version.txt should be accessible: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read /_metrics/version.txt: %v", err)
	}

	if string(content) != "v1" {
		t.Errorf("Expected 'v1', got %q", string(content))
	}

	if wildcardCalled {
		t.Error("Wildcard handler should NOT be called for /_metrics paths")
	}
}

// Test that /_metrics routes match even with a root / handler
func TestMetricsRouteWithRootHandler(t *testing.T) {
	fsys := ragfs.New()

	// Register a root handler AFTER metrics
	rootCalled := false
	fsys.Map("/", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		rootCalled = true
		return []fs.DirEntry{
			&testFileEntry{name: "root.txt", content: []byte("from root")},
		}, nil
	}))

	// Test that /_metrics/summary.md is accessible
	file, err := fsys.Open("/_metrics/summary.md")
	if err != nil {
		t.Fatalf("/_metrics/summary.md should be accessible: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read /_metrics/summary.md: %v", err)
	}

	if !strings.Contains(string(content), "ragfs Metrics Summary") {
		t.Error("Summary should contain 'ragfs Metrics Summary'")
	}

	if rootCalled {
		t.Error("Root handler should NOT be called for /_metrics paths")
	}
}

// Test all /_metrics subdirectory paths are accessible
func TestAllMetricsPathsAccessible(t *testing.T) {
	fsys := ragfs.New()

	// Add a wildcard to ensure metrics still work
	fsys.Map("/*", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "other.txt", content: []byte("other")},
		}, nil
	}))

	testCases := []struct {
		path        string
		shouldExist bool
	}{
		{"/_metrics", true},
		{"/_metrics/version.txt", true},
		{"/_metrics/summary.md", true},
		{"/_metrics/cache", true},
		{"/_metrics/cache/cache_hit_count", true},
		{"/_metrics/cache/cache_miss_count", true},
		{"/_metrics/cache/cache_hit_rate", true},
		{"/_metrics/cache/cache_entries", true},
		{"/_metrics/cache/cache_evictions", true},
		{"/_metrics/cache/summary.md", true},
		{"/_metrics/io", true},
		{"/_metrics/io/bytes_read", true},
		{"/_metrics/io/bytes_written", true},
		{"/_metrics/io/read_ops", true},
		{"/_metrics/io/write_ops", true},
		{"/_metrics/io/summary.md", true},
		{"/_metrics/errors", true},
		{"/_metrics/errors/error_count", true},
		{"/_metrics/errors/errors_by_type.json", true},
		{"/_metrics/errors/summary.md", true},
		{"/_metrics/system", true},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			file, err := fsys.Open(tc.path)
			if tc.shouldExist {
				if err != nil {
					t.Fatalf("Path %s should exist but got error: %v", tc.path, err)
				}
				file.Close()
			} else {
				if err == nil {
					file.Close()
					t.Fatalf("Path %s should not exist but was accessible", tc.path)
				}
			}
		})
	}
}

// Test that /_metrics has longest-match priority over shorter patterns
func TestMetricsLongestMatchPriority(t *testing.T) {
	fsys := ragfs.New()

	// Add various conflicting patterns
	fsys.Map("/", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return nil, fs.ErrNotExist
	}))

	fsys.Map("/*", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return nil, fs.ErrNotExist
	}))

	fsys.Map("/{path}/**", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return nil, fs.ErrNotExist
	}))

	// Despite all these patterns, /_metrics should still work
	file, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("/_metrics/version.txt should match despite other patterns: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(content) != "v1" {
		t.Errorf("Expected 'v1', got %q", string(content))
	}
}

// Test that /_metrics ReadDir works correctly
func TestMetricsReadDir(t *testing.T) {
	fsys := ragfs.New()

	// Add a wildcard handler
	fsys.Map("/*", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "wildcard.txt", content: []byte("wild")},
		}, nil
	}))

	// ReadDir on /_metrics should work
	entries, err := fsys.ReadDir("/_metrics")
	if err != nil {
		t.Fatalf("ReadDir(/_metrics) failed: %v", err)
	}

	// Should contain at least: ., .., version.txt, summary.md, cache, io, errors, system
	if len(entries) < 8 {
		t.Errorf("Expected at least 8 entries in /_metrics, got %d", len(entries))
	}

	// Check for expected entries
	expectedNames := map[string]bool{
		".":           false,
		"..":          false,
		"version.txt": false,
		"summary.md":  false,
		"cache":       false,
		"io":          false,
		"errors":      false,
		"system":      false,
	}

	for _, entry := range entries {
		if _, exists := expectedNames[entry.Name()]; exists {
			expectedNames[entry.Name()] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("Expected entry %q not found in /_metrics directory", name)
		}
	}
}

// Test that /_metrics subdirectory ReadDir works
func TestMetricsCacheReadDir(t *testing.T) {
	fsys := ragfs.New()

	entries, err := fsys.ReadDir("/_metrics/cache")
	if err != nil {
		t.Fatalf("ReadDir(/_metrics/cache) failed: %v", err)
	}

	// Should contain: ., .., summary.md, and the 5 cache metric files
	if len(entries) < 7 {
		t.Errorf("Expected at least 7 entries in /_metrics/cache, got %d", len(entries))
	}

	expectedNames := []string{
		"summary.md",
		"cache_hit_count",
		"cache_miss_count",
		"cache_hit_rate",
		"cache_entries",
		"cache_evictions",
	}

	found := make(map[string]bool)
	for _, entry := range entries {
		found[entry.Name()] = true
	}

	for _, name := range expectedNames {
		if !found[name] {
			t.Errorf("Expected file %q not found in /_metrics/cache", name)
		}
	}
}

// Test that /_metrics paths work correctly even with complex user routing patterns
func TestMetricsWithComplexUserRoutes(t *testing.T) {
	fsys := ragfs.New()

	// Register various user patterns that should NOT interfere with /_metrics
	userHandlerCalled := false

	// Pattern 1: Wildcard with parameter
	fsys.Map("/{resource}", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		userHandlerCalled = true
		return []fs.DirEntry{
			&testFileEntry{name: "user.txt", content: []byte("user resource")},
		}, nil
	}))

	// Pattern 2: Two-level wildcard
	fsys.Map("/{a}/{b}", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		userHandlerCalled = true
		return []fs.DirEntry{
			&testFileEntry{name: "file.txt", content: []byte("two-level")},
		}, nil
	}))

	// Pattern 3: Wildcard catch-all
	fsys.Map("/*", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		userHandlerCalled = true
		return []fs.DirEntry{
			&testFileEntry{name: "wildcard.txt", content: []byte("catch-all")},
		}, nil
	}))

	// /_metrics paths should still work and NOT call user handlers
	userHandlerCalled = false
	file, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("/_metrics/version.txt should be accessible despite user routes: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(content) != "v1" {
		t.Errorf("Expected 'v1', got %q", string(content))
	}

	if userHandlerCalled {
		t.Error("User handlers should NOT be called for /_metrics paths")
	}
}

// Test edge case: paths that start with _metrics but are not /_metrics
func TestNonMetricsPaths(t *testing.T) {
	fsys := ragfs.New()

	userHandlerCalled := false
	fsys.Map("/other/_metrics", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		userHandlerCalled = true
		return []fs.DirEntry{
			&testFileEntry{name: "_metrics", content: []byte("user data")},
		}, nil
	}))

	// /other/_metrics should call user handler
	file, err := fsys.Open("/other/_metrics")
	if err != nil {
		t.Fatalf("User path should work: %v", err)
	}
	defer file.Close()

	if !userHandlerCalled {
		t.Error("User handler should be called for /other/_metrics")
	}

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(content) != "user data" {
		t.Errorf("Expected 'user data', got %q", string(content))
	}
}

// Test that /_metrics has priority over deeply nested user patterns
func TestMetricsWithDeeplyNestedPatterns(t *testing.T) {
	fsys := ragfs.New()

	// Register deeply nested patterns
	fsys.Map("/{a}/{b}/{c}", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "nested.txt", content: []byte("nested three levels")},
		}, nil
	}))

	fsys.Map("/{x}/{y}/{z}/{w}", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "deeply.txt", content: []byte("nested four levels")},
		}, nil
	}))

	// /_metrics should still work despite these deeply nested patterns
	file, err := fsys.Open("/_metrics/cache/cache_hit_count")
	if err != nil {
		t.Fatalf("/_metrics/cache/cache_hit_count should be accessible: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	// Should get a numeric value
	if len(content) == 0 {
		t.Error("Expected non-empty cache hit count")
	}
}

// Test that metrics work with caching enabled
func TestMetricsWithCaching(t *testing.T) {
	fsys := ragfs.New()
	fsys.EnableCache(ragfs.CacheConfig{
		MaxEntries: 100,
		TTL:        60 * 1000, // 60 seconds
	})

	// Add a wildcard handler
	fsys.Map("/*", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "cached.txt", content: []byte("cached")},
		}, nil
	}))

	// First access - should work
	file1, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("First access failed: %v", err)
	}
	file1.Close()

	// Second access - should work (possibly from cache)
	file2, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("Second access failed: %v", err)
	}
	file2.Close()

	// Verify content is still correct
	file3, err := fsys.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("Third access failed: %v", err)
	}
	defer file3.Close()

	content, err := io.ReadAll(file3)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(content) != "v1" {
		t.Errorf("Expected 'v1', got %q", string(content))
	}
}
