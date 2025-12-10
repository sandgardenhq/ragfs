package ragfs

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
)

// Example demonstrates how to create a filesystem that maps paths to JSON data.
//
// This example shows:
//   - Creating a ragfs filesystem
//   - Mapping path patterns to handlers
//   - Extracting path parameters
//   - Converting data to filesystem entries
func Example() {
	// Sample configuration data
	config := map[string]any{
		"app": map[string]any{
			"name":    "MyApp",
			"version": "1.0.0",
		},
		"database": map[string]any{
			"host": "localhost",
			"port": 5432,
		},
	}

	// Create a new filesystem
	fsys := New()

	// Map a handler for /config/{key} pattern
	fsys.Map("/config/{key}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		key := params["key"]

		// Look up the value in our config
		value, exists := config[key]
		if !exists {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Convert to JSON
		content, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}

		// Return as a file entry
		return []fs.DirEntry{
			&exampleFileEntry{
				name:    key + ".json",
				content: content,
			},
		}, nil
	})

	// Now you can read from the filesystem
	// f, _ := fsys.Open("/config/database")
	// content, _ := io.ReadAll(f)
	// fmt.Println(string(content))
}

// EmailExample demonstrates fetching data with date parameters.
func EmailExample() {
	fsys := New()

	// Map emails by date
	fsys.Map("/emails/{date}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		date := params["date"]

		// In a real implementation, this would query a database or API
		emails := fetchEmailsForDate(date)

		// Return each email as a separate file entry
		var entries []fs.DirEntry
		for _, email := range emails {
			entries = append(entries, &exampleFileEntry{
				name:    email.ID + ".txt",
				content: []byte(email.Body),
			})
		}

		return entries, nil
	})

	// Read all emails for a specific date
	// f, _ := fsys.Open("/emails/2025-10-07")
}

// JSONPathExample shows navigating nested JSON structures.
func JSONPathExample() {
	data := map[string]any{
		"users": map[string]any{
			"alice": map[string]any{
				"email": "alice@example.com",
				"role":  "admin",
			},
			"bob": map[string]any{
				"email": "bob@example.com",
				"role":  "user",
			},
		},
	}

	fsys := New()

	// Generic handler that navigates JSON by path
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		// Split path into components
		parts := strings.Split(strings.Trim(path, "/"), "/")

		// Navigate through the JSON structure
		var current any = data
		for _, part := range parts {
			m, ok := current.(map[string]any)
			if !ok {
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
			current, ok = m[part]
			if !ok {
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
		}

		// Serialize the result
		content, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			return nil, err
		}

		return []fs.DirEntry{
			&exampleFileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil
	}

	// Map various paths
	fsys.Map("/users/alice", handler)
	fsys.Map("/users/bob", handler)
	fsys.Map("/users/alice/email", handler)

	// Reading /users/alice returns alice's full record
	// Reading /users/alice/email returns just the email
}

// exampleFileEntry implements fs.DirEntry with content support.
type exampleFileEntry struct {
	name    string
	content []byte
}

func (e *exampleFileEntry) Name() string               { return e.name }
func (e *exampleFileEntry) IsDir() bool                { return false }
func (e *exampleFileEntry) Type() fs.FileMode          { return 0 }
func (e *exampleFileEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *exampleFileEntry) Content() []byte            { return e.content }

// Email represents an email message (mock type for example).
type Email struct {
	ID   string
	Body string
}

// fetchEmailsForDate is a mock function that would fetch emails from a real source.
func fetchEmailsForDate(date string) []Email {
	return []Email{
		{ID: "msg1", Body: "Email content for " + date},
		{ID: "msg2", Body: "Another email for " + date},
	}
}
