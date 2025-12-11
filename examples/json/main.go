package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/brittcrawford/ragfs"
	fuseFS "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func main() {
	// Parse command-line flags
	mountPoint := flag.String("mount", "", "Mount point directory (required)")
	configFile := flag.String("config", "config.json", "JSON configuration file to expose")
	flag.Parse()

	if *mountPoint == "" {
		fmt.Println("Usage: ragfs-mount -mount <directory> [-config <file>]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load configuration file
	data, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	// Create ragfs filesystem
	fsys := ragfs.New()

	// Create a handler that maps filesystem paths to JSON paths
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		// Convert /config/name to ["config", "name"]
		// Handle root path specially
		trimmed := strings.Trim(path, "/")
		var parts []string
		if trimmed != "" {
			parts = strings.Split(trimmed, "/")
		}

		// Navigate through JSON
		var current any = config
		for _, part := range parts {
			if part == "" {
				continue
			}
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

		// Handle based on type: primitives are files, objects/arrays are directories
		switch v := current.(type) {
		case map[string]any:
			// Object - return directory listing
			var entries []fs.DirEntry
			for key, value := range v {
				// Determine if this child is a file or directory
				isDir := false
				switch value.(type) {
				case map[string]any, []any:
					isDir = true
				}
				entries = append(entries, &dirEntry{
					name:  key,
					isDir: isDir,
				})
			}
			return entries, nil

		case []any:
			// Array - return directory listing with numeric indices
			var entries []fs.DirEntry
			for i, value := range v {
				// Determine if this child is a file or directory
				isDir := false
				switch value.(type) {
				case map[string]any, []any:
					isDir = true
				}
				entries = append(entries, &dirEntry{
					name:  fmt.Sprintf("%d", i),
					isDir: isDir,
				})
			}
			return entries, nil

		case string:
			// Primitive string - return as file
			content := []byte(v)
			return []fs.DirEntry{
				&fileEntry{
					name:    parts[len(parts)-1],
					content: content,
				},
			}, nil

		case float64:
			// Primitive number - return as file
			content := []byte(fmt.Sprintf("%v", v))
			return []fs.DirEntry{
				&fileEntry{
					name:    parts[len(parts)-1],
					content: content,
				},
			}, nil

		case bool:
			// Primitive bool - return as file
			content := []byte(fmt.Sprintf("%v", v))
			return []fs.DirEntry{
				&fileEntry{
					name:    parts[len(parts)-1],
					content: content,
				},
			}, nil

		default:
			// Unknown type - return JSON representation
			content, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return nil, err
			}
			return []fs.DirEntry{
				&fileEntry{
					name:    parts[len(parts)-1],
					content: content,
				},
			}, nil
		}
	}

	// Map all possible paths (this is a simple example)
	// In a real implementation, you'd want more sophisticated routing
	fsys.Map("/*", handler)

	// Create FUSE root
	root := ragfs.NewFUSERoot(fsys)

	log.Printf("About to mount at %s", *mountPoint)

	// Mount the filesystem
	server, err := fuseFS.Mount(*mountPoint, root, &fuseFS.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			FsName:     "ragfs",
			Name:       "ragfs",
			AllowOther: false,
		},
	})
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}

	log.Printf("Mounted ragfs at %s", *mountPoint)
	log.Printf("Press Ctrl-C to unmount")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nUnmounting...")
		if err := server.Unmount(); err != nil {
			log.Printf("Unmount failed: %v", err)
		}
	}()

	// Wait for the filesystem to be unmounted
	server.Wait()
	fmt.Println("Filesystem unmounted")
}

type fileEntry struct {
	name    string
	content []byte
}

func (e *fileEntry) Name() string               { return e.name }
func (e *fileEntry) IsDir() bool                { return false }
func (e *fileEntry) Type() fs.FileMode          { return 0 }
func (e *fileEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *fileEntry) Content() []byte            { return e.content }

type dirEntry struct {
	name  string
	isDir bool
}

func (e *dirEntry) Name() string               { return e.name }
func (e *dirEntry) IsDir() bool                { return e.isDir }
func (e *dirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (e *dirEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *dirEntry) Content() []byte            { return nil }
