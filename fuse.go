package ragfs

import (
	"context"
	"io"
	"io/fs"
	"syscall"

	fuseFS "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FUSENode implements the go-fuse node interface for ragfs.
// It bridges between the ragfs Handler system and FUSE filesystem operations.
type FUSENode struct {
	fuseFS.Inode
	fsys  *FS
	path  string
	isDir bool
}

var _ fuseFS.NodeOpener = (*FUSENode)(nil)
var _ fuseFS.NodeGetattrer = (*FUSENode)(nil)
var _ fuseFS.NodeReaddirer = (*FUSENode)(nil)
var _ fuseFS.NodeLookuper = (*FUSENode)(nil)

// NewFUSERoot creates a new FUSE root node from a ragfs filesystem.
// The root node is always a directory.
func NewFUSERoot(fsys *FS) *FUSENode {
	return &FUSENode{
		fsys:  fsys,
		path:  "",
		isDir: true,
	}
}

// Getattr returns file attributes for the FUSE filesystem.
func (n *FUSENode) Getattr(ctx context.Context, fh fuseFS.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if n.isDir {
		// This is a directory node
		out.Mode = syscall.S_IFDIR | 0755
		out.Size = 0
		return fuseFS.OK
	}

	// This is a file node - try to open it to get the size
	f, err := n.fsys.Open(n.path)
	if err != nil {
		return syscall.ENOENT
	}
	defer f.Close()

	// Get file info for size
	stat, err := f.Stat()
	if err != nil {
		return syscall.EIO
	}

	// Set attributes for a regular file
	out.Mode = syscall.S_IFREG | 0444 // Read-only file
	out.Size = uint64(stat.Size())
	return fuseFS.OK
}

// Open opens a file in the FUSE filesystem.
func (n *FUSENode) Open(ctx context.Context, flags uint32) (fh fuseFS.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// Don't open directories
	if n.isDir {
		return nil, 0, syscall.EISDIR
	}

	// Open the file using ragfs
	f, err := n.fsys.Open(n.path)
	if err != nil {
		return nil, 0, syscall.ENOENT
	}

	return &FUSEFile{file: f}, fuse.FOPEN_DIRECT_IO, fuseFS.OK
}

// Readdir lists directory contents.
func (n *FUSENode) Readdir(ctx context.Context) (fuseFS.DirStream, syscall.Errno) {
	// Build path for this directory
	path := "/" + n.path
	if n.path == "" {
		path = "/"
	}

	// Call the handler to get directory entries
	entries, err := n.fsys.Open(path)
	if err != nil {
		return fuseFS.NewListDirStream(nil), syscall.ENOENT
	}
	defer entries.Close()

	// Try to read the directory entries
	if dirReader, ok := entries.(fs.ReadDirFile); ok {
		dirEntries, err := dirReader.ReadDir(-1)
		if err != nil {
			return fuseFS.NewListDirStream(nil), syscall.EIO
		}

		// Convert to fuse.DirEntry
		var fuseDirEntries []fuse.DirEntry
		for _, entry := range dirEntries {
			mode := uint32(syscall.S_IFREG)
			if entry.IsDir() {
				mode = syscall.S_IFDIR
			}
			fuseDirEntries = append(fuseDirEntries, fuse.DirEntry{
				Name: entry.Name(),
				Mode: mode,
			})
		}
		return fuseFS.NewListDirStream(fuseDirEntries), fuseFS.OK
	}

	return fuseFS.NewListDirStream(nil), fuseFS.OK
}

// Lookup looks up a child node by name.
func (n *FUSENode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fuseFS.Inode, syscall.Errno) {
	// Build child path (without leading slash for internal storage)
	var childPath string
	if n.path == "" {
		childPath = name
	} else {
		childPath = n.path + "/" + name
	}

	// Build path for ragfs (with leading slash)
	ragfsPath := "/" + childPath

	// Try to open the path
	f, err := n.fsys.Open(ragfsPath)
	if err != nil {
		// Path doesn't exist
		return nil, syscall.ENOENT
	}
	defer f.Close()

	// Check if it's a directory or file
	stat, err := f.Stat()
	if err != nil {
		return nil, syscall.EIO
	}

	isDir := stat.IsDir()

	child := &FUSENode{
		fsys:  n.fsys,
		path:  childPath, // Store without leading slash
		isDir: isDir,
	}

	if isDir {
		// Set entry attributes for directory
		out.Mode = syscall.S_IFDIR | 0755
		out.Size = 0
		return n.NewInode(ctx, child, fuseFS.StableAttr{Mode: syscall.S_IFDIR}), fuseFS.OK
	}

	// Set entry attributes for file
	out.Mode = syscall.S_IFREG | 0444
	out.Size = uint64(stat.Size())
	return n.NewInode(ctx, child, fuseFS.StableAttr{Mode: syscall.S_IFREG}), fuseFS.OK
}

// FUSEFile implements the FUSE file handle interface.
type FUSEFile struct {
	file fs.File
}

var _ fuseFS.FileReader = (*FUSEFile)(nil)
var _ fuseFS.FileReleaser = (*FUSEFile)(nil)

// Read reads data from the file.
func (f *FUSEFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	// Check if we can seek to the offset
	if seeker, ok := f.file.(interface {
		Seek(offset int64, whence int) (int64, error)
	}); ok {
		_, err := seeker.Seek(off, 0)
		if err != nil {
			return nil, syscall.EIO
		}
	}

	// Read data from the file
	buf := make([]byte, len(dest))
	n, err := f.file.Read(buf)
	if err != nil && err != io.EOF {
		return nil, syscall.EIO
	}

	// Return the data read (even if n=0 for EOF)
	return fuse.ReadResultData(buf[:n]), fuseFS.OK
}

// Release closes the file.
func (f *FUSEFile) Release(ctx context.Context) syscall.Errno {
	err := f.file.Close()
	if err != nil {
		return syscall.EIO
	}
	return fuseFS.OK
}
