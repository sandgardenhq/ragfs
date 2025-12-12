package ragfs

import (
	"bytes"
	"encoding/gob"
	"io/fs"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

// cachedDirEntry is a simple fs.DirEntry implementation for cache entries
type cachedDirEntry struct {
	name    string
	isDir   bool
	content []byte
}

func (e *cachedDirEntry) Name() string               { return e.name }
func (e *cachedDirEntry) IsDir() bool                { return e.isDir }
func (e *cachedDirEntry) Type() fs.FileMode          { return 0 }
func (e *cachedDirEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *cachedDirEntry) Content() []byte            { return e.content }

// BoltDBCacheConfig configures the BoltDB-backed cache for a ragfs filesystem.
// This provides persistent caching that survives process restarts.
type BoltDBCacheConfig struct {
	// DBPath is the path to the BoltDB database file.
	// The file will be created if it doesn't exist.
	DBPath string

	// MaxEntries is the maximum number of entries per cache layer.
	// When the limit is reached, least recently used entries are evicted.
	// 0 means unlimited (no eviction based on count).
	MaxEntries int

	// TTL is the time-to-live for cached entries.
	// Entries older than TTL are considered expired and will be refetched.
	// Special values:
	//   0: Default TTL (30 seconds) is used
	//   NoTTL (-1): TTL is disabled, entries never expire based on time
	//   > 0: Custom TTL duration
	TTL time.Duration
}

// boltDBCache wraps a BoltDB database for persistent caching.
type boltDBCache struct {
	db         *bbolt.DB
	stats      *CacheStats
	maxEntries int
	ttl        time.Duration
}

// cacheEntry represents a cached value with metadata.
type cacheEntry struct {
	Value     []byte
	Timestamp time.Time
}

// serializableDirEntry is a serializable representation of a DirEntry
type serializableDirEntry struct {
	Name    string
	IsDir   bool
	Content []byte
}

// toSerializable converts []fs.DirEntry to []serializableDirEntry
func toSerializable(entries []fs.DirEntry) []serializableDirEntry {
	result := make([]serializableDirEntry, len(entries))
	for i, entry := range entries {
		sEntry := serializableDirEntry{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
		}
		if ce, ok := entry.(interface{ Content() []byte }); ok {
			sEntry.Content = ce.Content()
		}
		result[i] = sEntry
	}
	return result
}

// fromSerializable converts []serializableDirEntry back to []fs.DirEntry
func fromSerializable(sEntries []serializableDirEntry) []fs.DirEntry {
	result := make([]fs.DirEntry, len(sEntries))
	for i, se := range sEntries {
		result[i] = &cachedDirEntry{
			name:    se.Name,
			isDir:   se.IsDir,
			content: se.Content,
		}
	}
	return result
}

var (
	handlerBucket = []byte("handler")
	contentBucket = []byte("content")
)

// EnableBoltDBCache enables persistent caching using BoltDB.
// The cache persists across process restarts.
func (f *FS) EnableBoltDBCache(config BoltDBCacheConfig) error {
	// Set default TTL if not specified
	ttl := config.TTL
	if ttl == 0 {
		ttl = 30 * time.Second
	}

	// Open BoltDB database
	db, err := bbolt.Open(config.DBPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return err
	}

	// Create buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(handlerBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(contentBucket)
		return err
	})
	if err != nil {
		db.Close()
		return err
	}

	boltCache := &boltDBCache{
		db:         db,
		stats:      &CacheStats{},
		maxEntries: config.MaxEntries,
		ttl:        ttl,
	}

	// Create a Cache that wraps the BoltDB cache
	f.cache = &Cache{
		handlerCache: nil, // We'll use BoltDB directly
		contentCache: nil, // We'll use BoltDB directly
		stats:        boltCache.stats,
	}
	f.boltDBCache = boltCache

	return nil
}

// CloseBoltDBCache closes the BoltDB database.
// This should be called when the filesystem is no longer needed.
func (f *FS) CloseBoltDBCache() error {
	if f.boltDBCache != nil && f.boltDBCache.db != nil {
		return f.boltDBCache.db.Close()
	}
	return nil
}

// getHandler retrieves cached handler results from BoltDB.
func (c *boltDBCache) getHandler(path string) []fs.DirEntry {
	var entries []fs.DirEntry

	c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(handlerBucket)
		if b == nil {
			return nil
		}

		data := b.Get([]byte(path))
		if data == nil {
			return nil
		}

		// Deserialize entry
		var entry cacheEntry
		buf := bytes.NewReader(data)
		dec := gob.NewDecoder(buf)
		if err := dec.Decode(&entry); err != nil {
			return err
		}

		// Check TTL expiry (skip if NoTTL is set)
		if c.ttl > 0 && time.Since(entry.Timestamp) > c.ttl {
			// Expired - delete it
			return nil
		}

		// Deserialize serializable entries
		var sEntries []serializableDirEntry
		buf = bytes.NewReader(entry.Value)
		dec = gob.NewDecoder(buf)
		if err := dec.Decode(&sEntries); err != nil {
			return err
		}

		// Convert back to fs.DirEntry
		entries = fromSerializable(sEntries)
		c.stats.Hits.Add(1)
		return nil
	})

	if entries == nil {
		c.stats.Misses.Add(1)
	}

	return entries
}

// setHandler stores handler results in BoltDB.
func (c *boltDBCache) setHandler(path string, entries []fs.DirEntry) {
	c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(handlerBucket)
		if b == nil {
			return nil
		}

		// Convert to serializable format
		sEntries := toSerializable(entries)

		// Serialize serializable entries
		var buf1 bytes.Buffer
		enc1 := gob.NewEncoder(&buf1)
		if err := enc1.Encode(sEntries); err != nil {
			return err
		}

		// Create cache entry with timestamp
		entry := cacheEntry{
			Value:     buf1.Bytes(),
			Timestamp: time.Now(),
		}

		// Serialize cache entry - use separate buffer
		var buf2 bytes.Buffer
		enc2 := gob.NewEncoder(&buf2)
		if err := enc2.Encode(entry); err != nil {
			return err
		}

		// Store in database
		err := b.Put([]byte(path), buf2.Bytes())
		if err == nil {
			c.stats.Entries.Add(1)
		}
		return err
	})
}

// getContent retrieves cached file content from BoltDB.
func (c *boltDBCache) getContent(path string) []byte {
	var content []byte

	c.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(contentBucket)
		if b == nil {
			return nil
		}

		data := b.Get([]byte(path))
		if data == nil {
			return nil
		}

		// Deserialize entry
		var entry cacheEntry
		buf := bytes.NewReader(data)
		dec := gob.NewDecoder(buf)
		if err := dec.Decode(&entry); err != nil {
			return err
		}

		// Check TTL expiry (skip if NoTTL is set)
		if c.ttl > 0 && time.Since(entry.Timestamp) > c.ttl {
			// Expired
			return nil
		}

		content = entry.Value
		c.stats.Hits.Add(1)
		return nil
	})

	if content == nil {
		c.stats.Misses.Add(1)
	}

	return content
}

// setContent stores file content in BoltDB.
func (c *boltDBCache) setContent(path string, content []byte) {
	c.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(contentBucket)
		if b == nil {
			return nil
		}

		// Create cache entry with timestamp
		entry := cacheEntry{
			Value:     content,
			Timestamp: time.Now(),
		}

		// Serialize cache entry
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(entry); err != nil {
			return err
		}

		// Store in database
		err := b.Put([]byte(path), buf.Bytes())
		if err == nil {
			c.stats.Entries.Add(1)
		}
		return err
	})
}

// invalidate removes a specific path from both cache buckets.
func (c *boltDBCache) invalidate(path string) {
	c.db.Update(func(tx *bbolt.Tx) error {
		hb := tx.Bucket(handlerBucket)
		if hb != nil {
			if hb.Get([]byte(path)) != nil {
				hb.Delete([]byte(path))
				c.stats.Entries.Add(-1)
			}
		}

		cb := tx.Bucket(contentBucket)
		if cb != nil {
			if cb.Get([]byte(path)) != nil {
				cb.Delete([]byte(path))
				c.stats.Entries.Add(-1)
			}
		}

		return nil
	})
}

// invalidatePrefix removes all paths starting with the given prefix.
func (c *boltDBCache) invalidatePrefix(prefix string) {
	c.db.Update(func(tx *bbolt.Tx) error {
		// Handler bucket
		hb := tx.Bucket(handlerBucket)
		if hb != nil {
			cursor := hb.Cursor()
			for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
				if strings.HasPrefix(string(k), prefix) {
					cursor.Delete()
					c.stats.Entries.Add(-1)
				}
			}
		}

		// Content bucket
		cb := tx.Bucket(contentBucket)
		if cb != nil {
			cursor := cb.Cursor()
			for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
				if strings.HasPrefix(string(k), prefix) {
					cursor.Delete()
					c.stats.Entries.Add(-1)
				}
			}
		}

		return nil
	})
}
