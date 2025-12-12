package ragfs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Test: Top-level summary.md format (Markdown table + JSON block)
func TestGenerateSummaryMarkdown(t *testing.T) {
	collector := NewCollector()

	// Record some sample metrics
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	collector.RecordCacheMiss()
	collector.RecordRead(1024, 5*time.Millisecond)
	collector.RecordWrite(512, 2*time.Millisecond)

	snapshot := collector.Snapshot()
	summary := GenerateSummaryMarkdown(snapshot)

	// Verify it contains markdown headers
	if !strings.Contains(summary, "# ragfs Metrics Summary") {
		t.Error("Summary should contain main header")
	}

	// Verify it contains table structure
	if !strings.Contains(summary, "| Metric | Value |") {
		t.Error("Summary should contain table header")
	}

	// Verify it contains cache metrics
	if !strings.Contains(summary, "Cache Hit Rate") {
		t.Error("Summary should contain cache hit rate")
	}

	// Verify it contains I/O metrics
	if !strings.Contains(summary, "Bytes Read") {
		t.Error("Summary should contain bytes read")
	}

	// Verify it contains JSON block
	if !strings.Contains(summary, "```json") {
		t.Error("Summary should contain JSON code block")
	}

	// Extract and parse JSON block
	jsonStart := strings.Index(summary, "```json")
	jsonEnd := strings.LastIndex(summary, "```")
	if jsonStart == -1 || jsonEnd == -1 || jsonStart >= jsonEnd {
		t.Fatal("Could not find valid JSON block")
	}

	jsonContent := summary[jsonStart+7 : jsonEnd]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &parsed); err != nil {
		t.Fatalf("JSON block should be valid JSON: %v", err)
	}

	// Verify JSON contains expected fields
	if _, ok := parsed["cache_hits"]; !ok {
		t.Error("JSON should contain cache_hits field")
	}
	if _, ok := parsed["bytes_read"]; !ok {
		t.Error("JSON should contain bytes_read field")
	}
}

// Test: Cache group summary.md format
func TestGenerateCacheSummaryMarkdown(t *testing.T) {
	collector := NewCollector()
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	collector.RecordCacheMiss()
	collector.UpdateCacheEntries(50)

	snapshot := collector.Snapshot()
	summary := GenerateCacheSummaryMarkdown(snapshot)

	// Verify header
	if !strings.Contains(summary, "# Cache Metrics") {
		t.Error("Cache summary should contain header")
	}

	// Verify cache-specific metrics
	if !strings.Contains(summary, "Cache Hits") {
		t.Error("Cache summary should contain hit count")
	}
	if !strings.Contains(summary, "Cache Misses") {
		t.Error("Cache summary should contain miss count")
	}
	if !strings.Contains(summary, "Hit Rate") {
		t.Error("Cache summary should contain hit rate")
	}

	// Verify JSON block exists
	if !strings.Contains(summary, "```json") {
		t.Error("Cache summary should contain JSON block")
	}
}

// Test: I/O group summary.md format
func TestGenerateIOSummaryMarkdown(t *testing.T) {
	collector := NewCollector()
	collector.RecordRead(1024, 5*time.Millisecond)
	collector.RecordRead(2048, 10*time.Millisecond)
	collector.RecordWrite(512, 2*time.Millisecond)

	snapshot := collector.Snapshot()
	summary := GenerateIOSummaryMarkdown(snapshot)

	// Verify header
	if !strings.Contains(summary, "# I/O Metrics") {
		t.Error("I/O summary should contain header")
	}

	// Verify I/O-specific metrics
	if !strings.Contains(summary, "Bytes Read") {
		t.Error("I/O summary should contain bytes read")
	}
	if !strings.Contains(summary, "Read Operations") {
		t.Error("I/O summary should contain read ops")
	}
	if !strings.Contains(summary, "Avg Read Latency") {
		t.Error("I/O summary should contain avg read latency")
	}

	// Verify JSON block exists
	if !strings.Contains(summary, "```json") {
		t.Error("I/O summary should contain JSON block")
	}
}

// Test: Error group summary.md format
func TestGenerateErrorSummaryMarkdown(t *testing.T) {
	collector := NewCollector()
	collector.RecordError(errTest("connection timeout"), "TimeoutError")
	collector.RecordError(errTest("auth failed"), "AuthError")

	snapshot := collector.Snapshot()
	summary := GenerateErrorSummaryMarkdown(snapshot)

	// Verify header
	if !strings.Contains(summary, "# Error Metrics") {
		t.Error("Error summary should contain header")
	}

	// Verify error-specific metrics
	if !strings.Contains(summary, "Total Errors") {
		t.Error("Error summary should contain total errors")
	}
	if !strings.Contains(summary, "Last Error") {
		t.Error("Error summary should contain last error")
	}

	// Verify JSON block exists
	if !strings.Contains(summary, "```json") {
		t.Error("Error summary should contain JSON block")
	}
}

// Test: RFC3339 timestamp formatting
func TestFormatTimestamp(t *testing.T) {
	testTime := time.Date(2025, 1, 15, 14, 30, 45, 0, time.UTC)
	formatted := FormatTimestamp(testTime)

	// Should be RFC3339 format
	expected := "2025-01-15T14:30:45Z"
	if formatted != expected {
		t.Errorf("Expected %s, got %s", expected, formatted)
	}

	// Verify zero time returns empty string or special value
	zeroFormatted := FormatTimestamp(time.Time{})
	if zeroFormatted != "never" {
		t.Errorf("Expected 'never' for zero time, got %s", zeroFormatted)
	}
}

// Test: Numeric formatting (integers vs floats)
func TestFormatNumeric(t *testing.T) {
	// Integer should format without decimals
	intFormatted := FormatInteger(1234567)
	if intFormatted != "1234567" {
		t.Errorf("Expected '1234567', got '%s'", intFormatted)
	}

	// Float should format with 3 decimal places
	floatFormatted := FormatFloat(123.456789)
	if floatFormatted != "123.457" {
		t.Errorf("Expected '123.457', got '%s'", floatFormatted)
	}

	// Zero should format correctly
	zeroFloat := FormatFloat(0.0)
	if zeroFloat != "0.000" {
		t.Errorf("Expected '0.000', got '%s'", zeroFloat)
	}
}

// Helper type for creating test errors
type errTest string

func (e errTest) Error() string {
	return string(e)
}
