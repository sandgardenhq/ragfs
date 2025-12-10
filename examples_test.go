package ragfs_test

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/brittcrawford/ragfs"
)

// TestJSONHandler demonstrates mapping filesystem paths to JSON queries.
// Path /config/name reads from data.config.name in the JSON
func TestJSONHandler(t *testing.T) {
	// Sample JSON data
	jsonData := `{
		"config": {
			"name": "MyApp",
			"version": "1.0.0",
			"database": {
				"host": "localhost",
				"port": 5432
			}
		},
		"users": ["alice", "bob", "charlie"]
	}`

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	fsys := ragfs.New()

	// Create a handler that maps filesystem paths to JSON paths
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		// Convert /config/name to ["config", "name"]
		parts := strings.Split(strings.Trim(path, "/"), "/")

		// Navigate through JSON
		var current any = data
		for _, part := range parts {
			switch v := current.(type) {
			case map[string]any:
				val, exists := v[part]
				if !exists {
					return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
				}
				current = val
			default:
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
		}

		// Convert the value to a file entry
		var content []byte
		switch v := current.(type) {
		case string:
			content = []byte(v)
		default:
			// For all other types (numbers, objects, arrays), return JSON
			var err error
			content, err = json.MarshalIndent(v, "", "  ")
			if err != nil {
				return nil, err
			}
		}

		// Return as a single file entry
		return []fs.DirEntry{
			&jsonFileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil
	}

	// Map a wildcard pattern (for now, we'll use specific paths)
	fsys.Map("/config/name", handler)
	fsys.Map("/config/version", handler)
	fsys.Map("/config/database/host", handler)
	fsys.Map("/config/database", handler)

	// Test reading /config/name
	t.Run("read config name", func(t *testing.T) {
		f, err := fsys.Open("/config/name")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if string(content) != "MyApp" {
			t.Errorf("expected 'MyApp', got %q", content)
		}
	})

	// Test reading nested object
	t.Run("read database config", func(t *testing.T) {
		f, err := fsys.Open("/config/database")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		var db map[string]any
		if err := json.Unmarshal(content, &db); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		if db["host"] != "localhost" {
			t.Errorf("expected host 'localhost', got %v", db["host"])
		}
	})
}

type jsonFileEntry struct {
	name    string
	content []byte
}

func (e *jsonFileEntry) Name() string               { return e.name }
func (e *jsonFileEntry) IsDir() bool                { return false }
func (e *jsonFileEntry) Type() fs.FileMode          { return 0 }
func (e *jsonFileEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *jsonFileEntry) Content() []byte            { return e.content }
