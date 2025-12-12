package ragfs

import (
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
)

// Test: Read /_metrics/version.txt
func TestMetricsFS_ReadVersionFile(t *testing.T) {
	collector := NewCollector()
	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("Failed to open version.txt: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read version.txt: %v", err)
	}

	if string(content) != "v1" {
		t.Errorf("Expected 'v1', got '%s'", string(content))
	}
}

// Test: Read /_metrics/summary.md
func TestMetricsFS_ReadSummaryFile(t *testing.T) {
	collector := NewCollector()
	collector.RecordCacheHit()
	collector.RecordRead(1024, 5*time.Millisecond)

	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/summary.md")
	if err != nil {
		t.Fatalf("Failed to open summary.md: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read summary.md: %v", err)
	}

	summary := string(content)
	if !strings.Contains(summary, "# ragfs Metrics Summary") {
		t.Error("Summary should contain header")
	}
	if !strings.Contains(summary, "```json") {
		t.Error("Summary should contain JSON block")
	}
}

// Test: Read /_metrics/cache/summary.md
func TestMetricsFS_ReadCacheSummary(t *testing.T) {
	collector := NewCollector()
	collector.RecordCacheHit()
	collector.RecordCacheMiss()

	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/cache/summary.md")
	if err != nil {
		t.Fatalf("Failed to open cache/summary.md: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read cache/summary.md: %v", err)
	}

	summary := string(content)
	if !strings.Contains(summary, "# Cache Metrics") {
		t.Error("Cache summary should contain header")
	}
}

// Test: Read single-value stat file /_metrics/cache/cache_hit_count
func TestMetricsFS_ReadStatFile(t *testing.T) {
	collector := NewCollector()
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	collector.RecordCacheHit()

	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/cache/cache_hit_count")
	if err != nil {
		t.Fatalf("Failed to open cache_hit_count: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read cache_hit_count: %v", err)
	}

	if strings.TrimSpace(string(content)) != "3" {
		t.Errorf("Expected '3', got '%s'", strings.TrimSpace(string(content)))
	}
}

// Test: Read JSON file /_metrics/errors/errors_by_type.json
func TestMetricsFS_ReadJSONFile(t *testing.T) {
	collector := NewCollector()
	collector.RecordError(errTest("timeout"), "TimeoutError")
	collector.RecordError(errTest("auth failed"), "AuthError")

	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/errors/errors_by_type.json")
	if err != nil {
		t.Fatalf("Failed to open errors_by_type.json: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read errors_by_type.json: %v", err)
	}

	jsonStr := string(content)
	if !strings.Contains(jsonStr, "TimeoutError") {
		t.Error("JSON should contain TimeoutError")
	}
	if !strings.Contains(jsonStr, "AuthError") {
		t.Error("JSON should contain AuthError")
	}
}

// Test: ReadDir on /_metrics/ directory
func TestMetricsFS_ReadDirRoot(t *testing.T) {
	collector := NewCollector()
	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics")
	if err != nil {
		t.Fatalf("Failed to open /_metrics: %v", err)
	}
	defer file.Close()

	dirFile, ok := file.(fs.ReadDirFile)
	if !ok {
		t.Fatal("/_metrics should be a directory")
	}

	entries, err := dirFile.ReadDir(-1)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	// Should contain: version.txt, summary.md, cache/, io/, system/, errors/
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

// Test: ReadDir on /_metrics/cache/ directory
func TestMetricsFS_ReadDirCache(t *testing.T) {
	collector := NewCollector()
	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/cache")
	if err != nil {
		t.Fatalf("Failed to open /_metrics/cache: %v", err)
	}
	defer file.Close()

	dirFile, ok := file.(fs.ReadDirFile)
	if !ok {
		t.Fatal("/_metrics/cache should be a directory")
	}

	entries, err := dirFile.ReadDir(-1)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	// Should contain: summary.md, cache_hit_count, cache_miss_count, etc.
	foundSummary := false
	foundHitCount := false
	for _, entry := range entries {
		if entry.Name() == "summary.md" {
			foundSummary = true
		}
		if entry.Name() == "cache_hit_count" {
			foundHitCount = true
		}
	}

	if !foundSummary {
		t.Error("Expected to find summary.md in /_metrics/cache")
	}
	if !foundHitCount {
		t.Error("Expected to find cache_hit_count in /_metrics/cache")
	}
}

// Test: Non-existent path returns error
func TestMetricsFS_NonExistentPath(t *testing.T) {
	collector := NewCollector()
	mfs := NewMetricsFS(collector)

	_, err := mfs.Open("/_metrics/nonexistent.txt")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// Test: FileInfo for stat files
func TestMetricsFS_StatFileInfo(t *testing.T) {
	collector := NewCollector()
	mfs := NewMetricsFS(collector)

	file, err := mfs.Open("/_metrics/version.txt")
	if err != nil {
		t.Fatalf("Failed to open version.txt: %v", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Name() != "version.txt" {
		t.Errorf("Expected name 'version.txt', got '%s'", info.Name())
	}

	if info.IsDir() {
		t.Error("version.txt should not be a directory")
	}

	if info.Size() <= 0 {
		t.Error("version.txt should have non-zero size")
	}
}
