# BoltDB Cache Example

This example demonstrates using the BoltDB persistent cache with ragfs. The cache survives process restarts, making it ideal for expensive operations like API calls.

## What This Example Does

Simulates an expensive API call (2 second delay) that fetches user data. The BoltDB cache stores the results persistently, so subsequent accesses are instant - even after restarting the program.

## Features Demonstrated

- **Persistent caching**: Cache survives process restarts
- **TTL support**: Entries expire after 5 minutes
- **Performance improvement**: 2 second API call becomes instant on cache hit
- **Cache statistics**: Monitor hits, misses, and entry count

## Running the Example

```bash
# First run - will make API calls (slow)
cd examples/boltdb-cache
go run main.go

# Second run - uses cache (instant!)
go run main.go
```

## Expected Output

### First Run
```
=== BoltDB Cache Example ===

BoltDB cache enabled: /tmp/ragfs-cache.db
TTL: 5 minutes

--- Test 1: First Access (Cache Miss) ---
[Handler Called #1] Fetching user: octocat
  Simulating expensive API call...

Response:
{
  "login": "octocat",
  ...
}

Time taken: 2.002s
Handler calls: 1

--- Test 2: Second Access (Cache Hit) ---
Response:
{
  "login": "octocat",
  ...
}

Time taken: 123µs (instant!)
Handler calls: 1 (same as before - cached!)
```

### Second Run (Process Restart)
The cache persists! All accesses will be instant because the data is already in the BoltDB cache from the previous run.

## Cache File

The cache is stored in `/tmp/ragfs-cache.db`. You can:
- Delete it to start fresh: `rm /tmp/ragfs-cache.db`
- Inspect it with BoltDB tools
- Share it across multiple processes

## Configuration

The example uses:
- **DBPath**: `/tmp/ragfs-cache.db`
- **MaxEntries**: 100 entries
- **TTL**: 5 minutes

Adjust these in `main.go` to suit your needs.

## Use Cases

Perfect for:
- Caching expensive API calls
- Reducing load on external services
- Improving response times for repeated queries
- Sharing cache across multiple processes
- Surviving service restarts without losing cache
