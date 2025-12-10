package ragfs

import (
	"context"
	"io/fs"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	fuseFS "github.com/hanwen/go-fuse/v2/fs"
)

// FUSENode implements the go-fuse node interface for ragfs.
// It bridges between the ragfs Handler system and FUSE filesystem operations.
type FUSENode struct {
	fuseFS.Inode
	fsys *FS
	path string
}

var _ fuseFS.NodeOpener = (*FUSENode)(nil)
var _ fuseFS.NodeGetattrer = (*FUSENode)(nil)
var _ fuseFS.NodeReaddirer = (*FUSENode)(nil)

// NewFUSERoot creates a new FUSE root node from a ragfs filesystem.
func NewFUSERoot(fsys *FS) *FUSENode {
	return &FUSENode{
		fsys: fsys,
		path: "/",
	}
}

// Getattr returns file attributes for the FUSE filesystem.
func (n *FUSENode) Getattr(ctx context.Context, fh fuseFS.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// Try to open the path to see if it exists
	_, err := n.fsys.Open(n.path)
	if err != nil {
		return syscall.ENOENT
	}

	// Set attributes for a regular file
	out.Mode = 0444 // Read-only file
	out.Size = 0    // Size will be determined when reading
	return fuseFS.OK
}

// Open opens a file in the FUSE filesystem.
func (n *FUSENode) Open(ctx context.Context, flags uint32) (fh fuseFS.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// Open the file using ragfs
	f, err := n.fsys.Open(n.path)
	if err != nil {
		return nil, 0, syscall.ENOENT
	}

	return &FUSEFile{file: f}, fuse.FOPEN_DIRECT_IO, fuseFS.OK
}

// Readdir lists directory contents.
func (n *FUSENode) Readdir(ctx context.Context) (fuseFS.DirStream, syscall.Errno) {
	// For now, return empty directory listing
	// This will be implemented when ReadDir support is added to ragfs
	return fuseFS.NewListDirStream(nil), fuseFS.OK
}

// Lookup looks up a child node by name.
func (n *FUSENode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fuseFS.Inode, syscall.Errno) {
	childPath := n.path + "/" + name
	if n.path == "/" {
		childPath = "/" + name
	}

	// Check if this path exists
	_, err := n.fsys.Open(childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	// Create a child node
	child := &FUSENode{
		fsys: n.fsys,
		path: childPath,
	}

	// Set entry attributes
	out.Mode = 0444
	out.Size = 0

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
	// For now, we'll read the entire file content
	// A better implementation would support seeking
	buf := make([]byte, len(dest))
	n, err := f.file.Read(buf)
	if err != nil {
		return nil, syscall.EIO
	}

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
