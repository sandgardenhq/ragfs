package ragfs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"testing"
	"time"
)

// BenchmarkCachedVsNonCached compares performance with and without caching
func BenchmarkCachedVsNonCached(b *testing.B) {
	// Simulate expensive handler with 1ms delay
	expensiveHandler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		time.Sleep(1 * time.Millisecond)
		return []fs.DirEntry{
			&testEntry{name: "data.txt", content: []byte("expensive result"), isDir: false},
		}, nil
	}

	b.Run("without-cache", func(b *testing.B) {
		fsys := New()
		fsys.Map("/data", expensiveHandler)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f, err := fsys.Open("/data")
			if err != nil {
				b.Fatal(err)
			}
			io.Copy(io.Discard, f)
			f.Close()
		}
	})

	b.Run("with-cache", func(b *testing.B) {
		fsys := New()
		fsys.EnableCache(CacheConfig{
			MaxEntries: 1000,
			TTL:        60 * time.Second,
		})
		fsys.Map("/data", expensiveHandler)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f, err := fsys.Open("/data")
			if err != nil {
				b.Fatal(err)
			}
			io.Copy(io.Discard, f)
			f.Close()
		}
	})
}

// BenchmarkCacheHit measures overhead of cache hit operations
func BenchmarkCacheHit(b *testing.B) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 1000,
		TTL:        60 * time.Second,
	})

	fsys.Map("/data", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testEntry{name: "data.txt", content: []byte("cached result"), isDir: false},
		}, nil
	})

	// Warm the cache
	f, _ := fsys.Open("/data")
	f.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := fsys.Open("/data")
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, f)
		f.Close()
	}
}

// BenchmarkCacheMiss measures overhead of cache miss operations
func BenchmarkCacheMiss(b *testing.B) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 1000,
		TTL:        60 * time.Second,
	})

	callCount := 0
	fsys.Map("/data/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{name: "data.txt", content: []byte(fmt.Sprintf("result-%s", params["id"])), isDir: false},
		}, nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("/data/%d", i)
		f, err := fsys.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, f)
		f.Close()
	}
}

// BenchmarkConcurrentReads measures performance under concurrent load
func BenchmarkConcurrentReads(b *testing.B) {
	scenarios := []struct {
		name       string
		goroutines int
		cached     bool
	}{
		{"sequential-nocache", 1, false},
		{"sequential-cached", 1, true},
		{"parallel-10-nocache", 10, false},
		{"parallel-10-cached", 10, true},
		{"parallel-100-nocache", 100, false},
		{"parallel-100-cached", 100, true},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			fsys := New()
			if sc.cached {
				fsys.EnableCache(CacheConfig{
					MaxEntries: 1000,
					TTL:        60 * time.Second,
				})
			}

			fsys.Map("/data", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
				// Simulate small work
				time.Sleep(100 * time.Microsecond)
				return []fs.DirEntry{
					&testEntry{name: "data.txt", content: []byte("result"), isDir: false},
				}, nil
			})

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					f, err := fsys.Open("/data")
					if err != nil {
						b.Fatal(err)
					}
					io.Copy(io.Discard, f)
					f.Close()
				}
			})
		})
	}
}

// BenchmarkCacheSizes compares performance with different cache sizes
func BenchmarkCacheSizes(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			fsys := New()
			fsys.EnableCache(CacheConfig{
				MaxEntries: size,
				TTL:        60 * time.Second,
			})

			fsys.Map("/data/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
				return []fs.DirEntry{
					&testEntry{
						name:    fmt.Sprintf("%s.txt", params["id"]),
						content: []byte(fmt.Sprintf("data-%s", params["id"])),
						isDir:   false,
					},
				}, nil
			})

			// Access pattern: cycle through 2x cache size
			paths := make([]string, size*2)
			for i := range paths {
				paths[i] = fmt.Sprintf("/data/%d", i)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				path := paths[i%len(paths)]
				f, err := fsys.Open(path)
				if err != nil {
					b.Fatal(err)
				}
				io.Copy(io.Discard, f)
				f.Close()
			}
		})
	}
}

// BenchmarkInvalidation measures performance of invalidation operations
func BenchmarkInvalidation(b *testing.B) {
	b.Run("invalidate-single", func(b *testing.B) {
		fsys := New()
		fsys.EnableCache(CacheConfig{
			MaxEntries: 1000,
			TTL:        60 * time.Second,
		})

		fsys.Map("/data/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{
				&testEntry{name: "data.txt", content: []byte("result"), isDir: false},
			}, nil
		})

		// Populate cache
		for i := 0; i < 100; i++ {
			f, _ := fsys.Open(fmt.Sprintf("/data/%d", i))
			f.Close()
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fsys.Invalidate(fmt.Sprintf("/data/%d", i%100))
		}
	})

	b.Run("invalidate-prefix", func(b *testing.B) {
		fsys := New()
		fsys.EnableCache(CacheConfig{
			MaxEntries: 1000,
			TTL:        60 * time.Second,
		})

		fsys.Map("/data/{category}/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{
				&testEntry{name: "data.txt", content: []byte("result"), isDir: false},
			}, nil
		})

		// Populate cache with different categories
		for cat := 0; cat < 10; cat++ {
			for i := 0; i < 10; i++ {
				f, _ := fsys.Open(fmt.Sprintf("/data/%d/%d", cat, i))
				f.Close()
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fsys.InvalidatePrefix(fmt.Sprintf("/data/%d/", i%10))
		}
	})
}

// BenchmarkTTLExpiry measures overhead of TTL checking
func BenchmarkTTLExpiry(b *testing.B) {
	scenarios := []struct {
		name string
		ttl  time.Duration
	}{
		{"no-ttl", 0},
		{"ttl-1s", 1 * time.Second},
		{"ttl-30s", 30 * time.Second},
		{"ttl-5m", 5 * time.Minute},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			fsys := New()
			fsys.EnableCache(CacheConfig{
				MaxEntries: 1000,
				TTL:        sc.ttl,
			})

			fsys.Map("/data", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
				return []fs.DirEntry{
					&testEntry{name: "data.txt", content: []byte("result"), isDir: false},
				}, nil
			})

			// Warm cache
			f, _ := fsys.Open("/data")
			f.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f, err := fsys.Open("/data")
				if err != nil {
					b.Fatal(err)
				}
				io.Copy(io.Discard, f)
				f.Close()
			}
		})
	}
}

// BenchmarkLRUEviction measures overhead of eviction operations
func BenchmarkLRUEviction(b *testing.B) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 100, // Small cache to trigger frequent evictions
		TTL:        60 * time.Second,
	})

	fsys.Map("/data/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testEntry{
				name:    fmt.Sprintf("%s.txt", params["id"]),
				content: []byte(fmt.Sprintf("data-%s", params["id"])),
				isDir:   false,
			},
		}, nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Access pattern that exceeds cache size
		path := fmt.Sprintf("/data/%d", i)
		f, err := fsys.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, f)
		f.Close()
	}

	b.StopTimer()
	stats := fsys.Stats()
	b.ReportMetric(float64(stats.Evictions.Load()), "evictions")
}

// BenchmarkContentCacheLayer measures Layer 2 (content) cache performance
func BenchmarkContentCacheLayer(b *testing.B) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 1000,
		TTL:        60 * time.Second,
	})

	// Handler returns large content to make content caching valuable
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	fsys.Map("/data", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testEntry{name: "large.bin", content: largeContent, isDir: false},
		}, nil
	})

	b.ResetTimer()
	b.SetBytes(int64(len(largeContent)))
	for i := 0; i < b.N; i++ {
		f, err := fsys.Open("/data")
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, f)
		f.Close()
	}
}

// BenchmarkStatsOverhead measures the overhead of stats tracking
func BenchmarkStatsOverhead(b *testing.B) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 1000,
		TTL:        60 * time.Second,
	})

	fsys.Map("/data", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testEntry{name: "data.txt", content: []byte("result"), isDir: false},
		}, nil
	})

	// Warm cache
	f, _ := fsys.Open("/data")
	f.Close()

	var wg sync.WaitGroup
	b.ResetTimer()

	// Concurrent reads + stats access
	for i := 0; i < b.N; i++ {
		wg.Add(2)

		// Goroutine 1: Read file
		go func() {
			defer wg.Done()
			f, _ := fsys.Open("/data")
			io.Copy(io.Discard, f)
			f.Close()
		}()

		// Goroutine 2: Read stats
		go func() {
			defer wg.Done()
			_ = fsys.Stats()
		}()

		wg.Wait()
	}
}
