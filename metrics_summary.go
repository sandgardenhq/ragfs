package ragfs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GenerateSummaryMarkdown generates the top-level summary.md with table and JSON.
func GenerateSummaryMarkdown(snapshot MetricsSnapshot) string {
	var sb strings.Builder

	sb.WriteString("# ragfs Metrics Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")

	// Cache metrics
	sb.WriteString(fmt.Sprintf("| Cache Hit Rate | %s |\n", FormatFloat(snapshot.CacheHitRate*100)+"%"))
	sb.WriteString(fmt.Sprintf("| Cache Hits | %s |\n", FormatInteger(snapshot.CacheHits)))
	sb.WriteString(fmt.Sprintf("| Cache Misses | %s |\n", FormatInteger(snapshot.CacheMisses)))
	sb.WriteString(fmt.Sprintf("| Cache Entries | %s |\n", FormatInteger(snapshot.CacheEntries)))

	// I/O metrics
	sb.WriteString(fmt.Sprintf("| Bytes Read | %s |\n", FormatInteger(snapshot.BytesRead)))
	sb.WriteString(fmt.Sprintf("| Bytes Written | %s |\n", FormatInteger(snapshot.BytesWritten)))
	sb.WriteString(fmt.Sprintf("| Read Operations | %s |\n", FormatInteger(snapshot.ReadOps)))
	sb.WriteString(fmt.Sprintf("| Write Operations | %s |\n", FormatInteger(snapshot.WriteOps)))

	// Error metrics
	sb.WriteString(fmt.Sprintf("| Total Errors | %s |\n", FormatInteger(snapshot.ErrorCount)))

	// Timestamps
	sb.WriteString(fmt.Sprintf("| Mounted At | %s |\n", FormatTimestamp(snapshot.MountedAt)))

	// Add JSON block
	sb.WriteString("\n## JSON Data\n\n")
	sb.WriteString("```json\n")

	// Create JSON object with all metrics
	jsonData := map[string]interface{}{
		"mounted_at":           FormatTimestamp(snapshot.MountedAt),
		"cache_hits":           snapshot.CacheHits,
		"cache_misses":         snapshot.CacheMisses,
		"cache_hit_rate":       snapshot.CacheHitRate,
		"cache_entries":        snapshot.CacheEntries,
		"cache_evictions":      snapshot.CacheEvictions,
		"bytes_read":           snapshot.BytesRead,
		"bytes_written":        snapshot.BytesWritten,
		"read_ops":             snapshot.ReadOps,
		"write_ops":            snapshot.WriteOps,
		"avg_read_latency_ms":  snapshot.AvgReadLatencyMs,
		"avg_write_latency_ms": snapshot.AvgWriteLatencyMs,
		"error_count":          snapshot.ErrorCount,
	}

	jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
	sb.Write(jsonBytes)
	sb.WriteString("\n```\n")

	return sb.String()
}

// GenerateCacheSummaryMarkdown generates the cache group summary.md.
func GenerateCacheSummaryMarkdown(snapshot MetricsSnapshot) string {
	var sb strings.Builder

	sb.WriteString("# Cache Metrics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Cache Hits | %s |\n", FormatInteger(snapshot.CacheHits)))
	sb.WriteString(fmt.Sprintf("| Cache Misses | %s |\n", FormatInteger(snapshot.CacheMisses)))
	sb.WriteString(fmt.Sprintf("| Hit Rate | %s |\n", FormatFloat(snapshot.CacheHitRate*100)+"%"))
	sb.WriteString(fmt.Sprintf("| Cache Entries | %s |\n", FormatInteger(snapshot.CacheEntries)))
	sb.WriteString(fmt.Sprintf("| Cache Evictions | %s |\n", FormatInteger(snapshot.CacheEvictions)))
	sb.WriteString(fmt.Sprintf("| Cache Capacity | %s |\n", FormatInteger(snapshot.CacheCapacity)))
	sb.WriteString(fmt.Sprintf("| Cache TTL (seconds) | %s |\n", FormatInteger(snapshot.CacheTTLSeconds)))

	// Add JSON block
	sb.WriteString("\n## JSON Data\n\n")
	sb.WriteString("```json\n")

	jsonData := map[string]interface{}{
		"cache_hits":        snapshot.CacheHits,
		"cache_misses":      snapshot.CacheMisses,
		"cache_hit_rate":    snapshot.CacheHitRate,
		"cache_entries":     snapshot.CacheEntries,
		"cache_evictions":   snapshot.CacheEvictions,
		"cache_capacity":    snapshot.CacheCapacity,
		"cache_ttl_seconds": snapshot.CacheTTLSeconds,
	}

	jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
	sb.Write(jsonBytes)
	sb.WriteString("\n```\n")

	return sb.String()
}

// GenerateIOSummaryMarkdown generates the I/O group summary.md.
func GenerateIOSummaryMarkdown(snapshot MetricsSnapshot) string {
	var sb strings.Builder

	sb.WriteString("# I/O Metrics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Bytes Read | %s |\n", FormatInteger(snapshot.BytesRead)))
	sb.WriteString(fmt.Sprintf("| Bytes Written | %s |\n", FormatInteger(snapshot.BytesWritten)))
	sb.WriteString(fmt.Sprintf("| Read Operations | %s |\n", FormatInteger(snapshot.ReadOps)))
	sb.WriteString(fmt.Sprintf("| Write Operations | %s |\n", FormatInteger(snapshot.WriteOps)))
	sb.WriteString(fmt.Sprintf("| Avg Read Latency (ms) | %s |\n", FormatFloat(snapshot.AvgReadLatencyMs)))
	sb.WriteString(fmt.Sprintf("| Avg Write Latency (ms) | %s |\n", FormatFloat(snapshot.AvgWriteLatencyMs)))
	sb.WriteString(fmt.Sprintf("| Last Read Time | %s |\n", FormatTimestamp(snapshot.LastReadTime)))
	sb.WriteString(fmt.Sprintf("| Last Write Time | %s |\n", FormatTimestamp(snapshot.LastWriteTime)))

	// Add JSON block
	sb.WriteString("\n## JSON Data\n\n")
	sb.WriteString("```json\n")

	jsonData := map[string]interface{}{
		"bytes_read":           snapshot.BytesRead,
		"bytes_written":        snapshot.BytesWritten,
		"read_ops":             snapshot.ReadOps,
		"write_ops":            snapshot.WriteOps,
		"avg_read_latency_ms":  snapshot.AvgReadLatencyMs,
		"avg_write_latency_ms": snapshot.AvgWriteLatencyMs,
		"last_read_time":       FormatTimestamp(snapshot.LastReadTime),
		"last_write_time":      FormatTimestamp(snapshot.LastWriteTime),
	}

	jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
	sb.Write(jsonBytes)
	sb.WriteString("\n```\n")

	return sb.String()
}

// GenerateErrorSummaryMarkdown generates the error group summary.md.
func GenerateErrorSummaryMarkdown(snapshot MetricsSnapshot) string {
	var sb strings.Builder

	sb.WriteString("# Error Metrics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Errors | %s |\n", FormatInteger(snapshot.ErrorCount)))
	sb.WriteString(fmt.Sprintf("| Last Error Time | %s |\n", FormatTimestamp(snapshot.LastErrorTime)))
	sb.WriteString(fmt.Sprintf("| Last Error | %s |\n", snapshot.LastErrorMessage))

	// Add JSON block
	sb.WriteString("\n## JSON Data\n\n")
	sb.WriteString("```json\n")

	jsonData := map[string]interface{}{
		"error_count":        snapshot.ErrorCount,
		"last_error_time":    FormatTimestamp(snapshot.LastErrorTime),
		"last_error_message": snapshot.LastErrorMessage,
		"errors_by_type":     snapshot.ErrorsByType,
		"recent_errors":      snapshot.RecentErrors,
	}

	jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
	sb.Write(jsonBytes)
	sb.WriteString("\n```\n")

	return sb.String()
}

// FormatTimestamp formats a time.Time as RFC3339, or "never" if zero.
func FormatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

// FormatInteger formats an int64 as a string.
func FormatInteger(n int64) string {
	return fmt.Sprintf("%d", n)
}

// FormatFloat formats a float64 with 3 decimal places.
func FormatFloat(f float64) string {
	return fmt.Sprintf("%.3f", f)
}
