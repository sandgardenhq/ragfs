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
	uid   uint32
	gid   uint32
}

var _ fuseFS.NodeOpener = (*FUSENode)(nil)
var _ fuseFS.NodeGetattrer = (*FUSENode)(nil)
var _ fuseFS.NodeReaddirer = (*FUSENode)(nil)
var _ fuseFS.NodeLookuper = (*FUSENode)(nil)
var _ fuseFS.NodeCreater = (*FUSENode)(nil)
var _ fuseFS.NodeUnlinker = (*FUSENode)(nil)
var _ fuseFS.NodeRenamer = (*FUSENode)(nil)

// NewFUSERoot creates a new FUSE root node from a ragfs filesystem.
// The root node is always a directory.
// uid and gid specify the owner of all files and directories in the filesystem.
func NewFUSERoot(fsys *FS, uid, gid uint32) *FUSENode {
	return &FUSENode{
		fsys:  fsys,
		path:  "",
		isDir: true,
		uid:   uid,
		gid:   gid,
	}
}

// Getattr returns file attributes for the FUSE filesystem.
func (n *FUSENode) Getattr(ctx context.Context, fh fuseFS.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if n.isDir {
		// This is a directory node
		out.Mode = syscall.S_IFDIR | 0755
		out.Size = 0
		out.Uid = n.uid
		out.Gid = n.gid
		return fuseFS.OK
	}

	// Build the ragfs path (with leading slash)
	ragfsPath := "/" + n.path

	// This is a file node - try to open it to get the size
	f, err := n.fsys.Open(ragfsPath)
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
	out.Mode = syscall.S_IFREG | 0644 // Read-write file
	out.Size = uint64(stat.Size())
	out.Uid = n.uid
	out.Gid = n.gid
	return fuseFS.OK
}

// Open opens a file in the FUSE filesystem.
func (n *FUSENode) Open(ctx context.Context, flags uint32) (fh fuseFS.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// Don't open directories
	if n.isDir {
		return nil, 0, syscall.EISDIR
	}

	// Build full path for ragfs (with leading slash)
	var ragfsPath string
	if n.path == "" {
		ragfsPath = "/"
	} else {
		ragfsPath = "/" + n.path
	}

	// Open the file using ragfs
	f, err := n.fsys.Open(ragfsPath)
	if err != nil {
		return nil, 0, syscall.ENOENT
	}

	return &FUSEFile{file: f, path: ragfsPath, fsys: n.fsys}, fuse.FOPEN_DIRECT_IO, fuseFS.OK
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
		uid:   n.uid,
		gid:   n.gid,
	}

	if isDir {
		// Set entry attributes for directory
		out.Mode = syscall.S_IFDIR | 0755
		out.Size = 0
		out.Uid = n.uid
		out.Gid = n.gid
		return n.NewInode(ctx, child, fuseFS.StableAttr{Mode: syscall.S_IFDIR}), fuseFS.OK
	}

	// Set entry attributes for file
	out.Mode = syscall.S_IFREG | 0644 // Read-write file
	out.Size = uint64(stat.Size())
	out.Uid = n.uid
	out.Gid = n.gid
	return n.NewInode(ctx, child, fuseFS.StableAttr{Mode: syscall.S_IFREG}), fuseFS.OK
}

// Create creates a new file in the filesystem.
func (n *FUSENode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *fuseFS.Inode, fh fuseFS.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	// Build the new file path
	var filePath string
	if n.path == "" {
		filePath = "/" + name
	} else {
		filePath = "/" + n.path + "/" + name
	}

	// Initialize the file with empty content so subsequent Getattr calls succeed
	err := n.fsys.WriteFile(filePath, []byte{})
	if err != nil {
		return nil, nil, 0, syscall.EIO
	}

	// Create a child node for the new file
	var childPath string
	if n.path == "" {
		childPath = name
	} else {
		childPath = n.path + "/" + name
	}

	child := &FUSENode{
		fsys:  n.fsys,
		path:  childPath,
		isDir: false,
		uid:   n.uid,
		gid:   n.gid,
	}

	// Set entry attributes
	out.Mode = syscall.S_IFREG | 0644
	out.Size = 0
	out.Uid = n.uid
	out.Gid = n.gid

	// Return a file handle for subsequent writes
	childInode := n.NewInode(ctx, child, fuseFS.StableAttr{Mode: syscall.S_IFREG})
	// Return FOPEN_DIRECT_IO to bypass page cache and enable writes
	return childInode, &FUSEFile{file: nil, path: filePath, fsys: n.fsys}, fuse.FOPEN_DIRECT_IO, fuseFS.OK
}

// Unlink removes a file from the filesystem.
func (n *FUSENode) Unlink(ctx context.Context, name string) syscall.Errno {
	// Build the file path
	var filePath string
	if n.path == "" {
		filePath = "/" + name
	} else {
		filePath = "/" + n.path + "/" + name
	}

	// Remove the file
	err := n.fsys.Remove(filePath)
	if err != nil {
		return syscall.EIO
	}

	return fuseFS.OK
}

// Rename renames a file in the filesystem.
func (n *FUSENode) Rename(ctx context.Context, name string, newParent fuseFS.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	// Build old path
	var oldPath string
	if n.path == "" {
		oldPath = "/" + name
	} else {
		oldPath = "/" + n.path + "/" + name
	}

	// Build new path
	newParentNode, ok := newParent.(*FUSENode)
	if !ok {
		return syscall.EIO
	}

	var newPath string
	if newParentNode.path == "" {
		newPath = "/" + newName
	} else {
		newPath = "/" + newParentNode.path + "/" + newName
	}

	// Rename the file
	err := n.fsys.Rename(oldPath, newPath)
	if err != nil {
		return syscall.EIO
	}

	return fuseFS.OK
}

// FUSEFile implements the FUSE file handle interface.
type FUSEFile struct {
	file fs.File
	path string
	fsys *FS
}

var _ fuseFS.FileReader = (*FUSEFile)(nil)
var _ fuseFS.FileWriter = (*FUSEFile)(nil)
var _ fuseFS.FileReleaser = (*FUSEFile)(nil)

// Read reads data from the file.
func (f *FUSEFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	// If file handle is nil, we need to re-open the file for reading
	if f.file == nil {
		file, err := f.fsys.Open(f.path)
		if err != nil {
			return nil, syscall.ENOENT
		}
		f.file = file
	}

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

// Write writes data to the file.
func (f *FUSEFile) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	// Write the data using the ragfs WriteFile API
	// Note: currently only supports full overwrites, not partial writes at offsets
	err := f.fsys.WriteFile(f.path, data)
	if err != nil {
		return 0, syscall.EIO
	}

	return uint32(len(data)), fuseFS.OK
}

// Release closes the file.
func (f *FUSEFile) Release(ctx context.Context) syscall.Errno {
	// Only close if we have an actual file handle
	if f.file != nil {
		err := f.file.Close()
		if err != nil {
			return syscall.EIO
		}
	}
	return fuseFS.OK
}
