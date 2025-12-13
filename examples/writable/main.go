package main

import (
	"context"
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

// InMemoryHandler stores files and directories in memory with full write support
type InMemoryHandler struct {
	ragfs.DefaultHandler
	files map[string][]byte
	dirs  map[string]bool
}

func NewInMemoryHandler() *InMemoryHandler {
	return &InMemoryHandler{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (h *InMemoryHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	// Handle root directory listing
	if path == "/" {
		var entries []fs.DirEntry
		// Add files in root
		for filePath, data := range h.files {
			// Only include files directly in root (not subdirectories)
			if strings.HasPrefix(filePath, "/") && !strings.Contains(filePath[1:], "/") {
				fileName := strings.TrimPrefix(filePath, "/")
				entries = append(entries, ragfs.NewFileEntry(fileName, data))
			}
		}
		// Add directories in root
		for dirPath := range h.dirs {
			// Only include directories directly in root
			if strings.HasPrefix(dirPath, "/") && !strings.Contains(dirPath[1:], "/") {
				dirName := strings.TrimPrefix(dirPath, "/")
				entries = append(entries, ragfs.NewDirEntry(dirName, true))
			}
		}
		return ragfs.NewDirectoryListing(entries), nil
	}

	// Check if it's a directory
	if h.dirs[path] {
		// List contents of directory
		var entries []fs.DirEntry
		prefix := path + "/"
		for filePath, data := range h.files {
			if strings.HasPrefix(filePath, prefix) {
				rest := filePath[len(prefix):]
				if !strings.Contains(rest, "/") {
					entries = append(entries, ragfs.NewFileEntry(rest, data))
				}
			}
		}
		for dirPath := range h.dirs {
			if strings.HasPrefix(dirPath, prefix) {
				rest := dirPath[len(prefix):]
				if !strings.Contains(rest, "/") {
					entries = append(entries, ragfs.NewDirEntry(rest, true))
				}
			}
		}
		return ragfs.NewDirectoryListing(entries), nil
	}

	// Handle individual file reads
	data, ok := h.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
	}

	// Return file entry with content
	return []fs.DirEntry{
		ragfs.NewFileEntry(path, data),
	}, nil
}

func (h *InMemoryHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
	h.files[path] = data
	log.Printf("WRITE: Wrote %d bytes to %s", len(data), path)
	return nil
}

func (h *InMemoryHandler) Remove(ctx context.Context, path string, params map[string]string) error {
	if _, ok := h.files[path]; !ok {
		return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrNotExist}
	}
	delete(h.files, path)
	log.Printf("REMOVE: Deleted %s", path)
	return nil
}

func (h *InMemoryHandler) Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error {
	data, ok := h.files[oldPath]
	if !ok {
		return &fs.PathError{Op: "rename", Path: oldPath, Err: fs.ErrNotExist}
	}
	h.files[newPath] = data
	delete(h.files, oldPath)
	log.Printf("RENAME: Renamed %s to %s", oldPath, newPath)
	return nil
}

func (h *InMemoryHandler) Mkdir(ctx context.Context, path string, params map[string]string) error {
	if h.dirs[path] {
		return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrExist}
	}
	h.dirs[path] = true
	log.Printf("MKDIR: Created directory %s", path)
	return nil
}

func (h *InMemoryHandler) Rmdir(ctx context.Context, path string, params map[string]string) error {
	if !h.dirs[path] {
		return &fs.PathError{Op: "rmdir", Path: path, Err: fs.ErrNotExist}
	}
	delete(h.dirs, path)
	log.Printf("RMDIR: Removed directory %s", path)
	return nil
}

func (h *InMemoryHandler) Truncate(ctx context.Context, path string, size int64, params map[string]string) error {
	data, ok := h.files[path]
	if !ok {
		return &fs.PathError{Op: "truncate", Path: path, Err: fs.ErrNotExist}
	}

	if int64(len(data)) > size {
		h.files[path] = data[:size]
	} else if int64(len(data)) < size {
		newData := make([]byte, size)
		copy(newData, data)
		h.files[path] = newData
	}
	log.Printf("TRUNCATE: Set size of %s to %d bytes", path, size)
	return nil
}

func main() {
	mountPoint := flag.String("mount", "/tmp/writable-test", "Mount point")
	flag.Parse()

	// Create filesystem
	fsys := ragfs.New()
	handler := NewInMemoryHandler()

	// Map root directory and all paths to the writable handler
	// The /* pattern matches files like /test.txt but NOT the root /
	// So we need to map both patterns plus /** for nested paths
	fsys.Map("/", handler)   // For listing root directory
	fsys.Map("/*", handler)  // For accessing files in root
	fsys.Map("/**", handler) // For accessing subdirectories and nested files

	log.Printf("Mounting writable filesystem at %s", *mountPoint)

	// Get current user's UID/GID
	uid := uint32(syscall.Getuid())
	gid := uint32(syscall.Getgid())

	// Create FUSE root with current user's UID/GID
	root := ragfs.NewFUSERoot(fsys, uid, gid)

	// Mount the filesystem with current user's permissions
	// AllowOther is required for write operations on macOS
	server, err := fuseFS.Mount(*mountPoint, root, &fuseFS.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			FsName:     "ragfs-writable",
			Name:       "ragfs-writable",
			AllowOther: false,
		},
	})
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}

	log.Println("Filesystem mounted successfully!")
	log.Println("You can now:")
	log.Println("  - Write files:  echo 'content' > " + *mountPoint + "/test.txt")
	log.Println("  - Read files:   cat " + *mountPoint + "/test.txt")
	log.Println("  - Rename:       mv " + *mountPoint + "/test.txt " + *mountPoint + "/renamed.txt")
	log.Println("  - Remove:       rm " + *mountPoint + "/test.txt")
	log.Println("  - Create dir:   mkdir " + *mountPoint + "/mydir")
	log.Println("  - Remove dir:   rmdir " + *mountPoint + "/mydir")
	log.Println("  - Truncate:     truncate -s 5 " + *mountPoint + "/test.txt")
	log.Println("")
	log.Println("Press Ctrl-C to unmount")

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
