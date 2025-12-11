# FUSE Integration Guide

This guide explains how to use ragfs as a FUSE (Filesystem in Userspace) plugin, allowing you to mount virtual filesystems that dynamically fetch data from various sources.

## Prerequisites

### macOS

Install macFUSE (formerly OSXFUSE):

```bash
brew install --cask macfuse
```

After installation, you may need to restart your system and grant system extension permissions in System Settings > Privacy & Security.

### Linux

Install FUSE library:

```bash
# Ubuntu/Debian
sudo apt-get install fuse libfuse-dev

# Fedora/RHEL
sudo dnf install fuse fuse-devel

# Arch Linux
sudo pacman -S fuse2
```

## Building the FUSE Mount Program

The example mount program is located in `examples/json/`:

```bash
# Build the mount program
go build -o ragfs-mount ./examples/json

# Or install it to your $GOPATH/bin
go install ./examples/json
```

## Using the Example Mount Program

The example program mounts a JSON configuration file as a virtual filesystem:

```bash
# Create a mount point
mkdir /tmp/ragfs-mount

# Mount the filesystem
./ragfs-mount -mount /tmp/ragfs-mount -config config.json

# In another terminal, access the files
cat /tmp/ragfs-mount/app/name
# Output: MyApp

cat /tmp/ragfs-mount/database/host
# Output: localhost

# View the entire database configuration
cat /tmp/ragfs-mount/database
# Output: JSON object with host, port, name, user

# Unmount by pressing Ctrl-C in the mount terminal
```

## Creating Your Own FUSE Plugin

### Basic Structure

```go
package main

import (
    "context"
    "io/fs"
    "log"

    "github.com/brittcrawford/ragfs"
    fuseFS "github.com/hanwen/go-fuse/v2/fs"
    "github.com/hanwen/go-fuse/v2/fuse"
)

func main() {
    // 1. Create a ragfs filesystem
    fsys := ragfs.New()

    // 2. Register your handlers
    fsys.Map("/data/{key}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
        // Fetch your data here
        content := fetchData(params["key"])

        return []fs.DirEntry{
            &FileEntry{
                name:    params["key"] + ".txt",
                content: []byte(content),
            },
        }, nil
    })

    // 3. Create FUSE root
    root := ragfs.NewFUSERoot(fsys)

    // 4. Mount the filesystem
    server, err := fuseFS.Mount("/path/to/mount", root, &fuseFS.Options{
        MountOptions: fuse.MountOptions{
            FsName: "myfs",
            Name:   "myfs",
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // 5. Wait for unmount
    server.Wait()
}
```

### Implementing File Entries

Your handler must return directory entries that implement the `Content() []byte` method:

```go
type FileEntry struct {
    name    string
    content []byte
}

func (e *FileEntry) Name() string               { return e.name }
func (e *FileEntry) IsDir() bool                { return false }
func (e *FileEntry) Type() fs.FileMode          { return 0 }
func (e *FileEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *FileEntry) Content() []byte            { return e.content }
```

## Advanced Use Cases

### Database-Backed Filesystem

```go
fsys.Map("/users/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    user, err := db.GetUser(params["id"])
    if err != nil {
        return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
    }

    data, _ := json.Marshal(user)
    return []fs.DirEntry{
        &FileEntry{name: params["id"] + ".json", content: data},
    }, nil
})
```

### API-Backed Filesystem

```go
fsys.Map("/repos/{owner}/{name}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    repo, err := githubClient.GetRepo(params["owner"], params["name"])
    if err != nil {
        return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
    }

    data, _ := json.Marshal(repo)
    return []fs.DirEntry{
        &FileEntry{name: "repo.json", content: data},
    }, nil
})
```

### Time-Based Data

```go
fsys.Map("/logs/{date}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    logs := fetchLogsForDate(params["date"])

    var entries []fs.DirEntry
    for _, log := range logs {
        entries = append(entries, &FileEntry{
            name:    log.ID + ".log",
            content: []byte(log.Message),
        })
    }

    return entries, nil
})
```

## Installation as System Service

### systemd (Linux)

Create `/etc/systemd/system/ragfs.service`:

```ini
[Unit]
Description=ragfs FUSE Mount
After=network.target

[Service]
Type=simple
User=youruser
ExecStart=/usr/local/bin/ragfs-mount -mount /mnt/ragfs -config /etc/ragfs/config.json
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable ragfs
sudo systemctl start ragfs
```

### launchd (macOS)

Create `~/Library/LaunchAgents/com.ragfs.mount.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ragfs.mount</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/ragfs-mount</string>
        <string>-mount</string>
        <string>/Users/youruser/ragfs</string>
        <string>-config</string>
        <string>/Users/youruser/.ragfs/config.json</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Load:

```bash
launchctl load ~/Library/LaunchAgents/com.ragfs.mount.plist
```

## Compilation Options

### Static Binary

For easier distribution, compile as a static binary:

```bash
CGO_ENABLED=1 go build -ldflags '-s -w -extldflags "-static"' -o ragfs-mount ./examples/json
```

### Cross-Compilation

Note: FUSE requires CGO, so cross-compilation is more complex:

```bash
# For Linux from macOS (requires cross-compiler)
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-linux-gnu-gcc go build -o ragfs-mount-linux ./examples/json
```

## Debugging

Enable FUSE debug output:

```go
server, err := fuseFS.Mount(mountPoint, root, &fuseFS.Options{
    MountOptions: fuse.MountOptions{
        Debug: true,  // Enable debug logging
    },
})
```

## Permissions

### User Permissions

By default, FUSE mounts are only accessible by the mounting user. To allow other users:

```go
MountOptions: fuse.MountOptions{
    AllowOther: true,
}
```

Note: This requires `user_allow_other` in `/etc/fuse.conf` (Linux) or appropriate macFUSE configuration (macOS).

### File Permissions

Customize file permissions in your FUSE node:

```go
func (n *FUSENode) Getattr(ctx context.Context, fh fuseFS.FileHandle, out *fuse.AttrOut) syscall.Errno {
    out.Mode = 0644  // rw-r--r--
    return fuseFS.OK
}
```

## Troubleshooting

### Mount fails with "Transport endpoint not connected"

The mount point may already be in use:

```bash
# Unmount first
fusermount -u /tmp/ragfs-mount  # Linux
umount /tmp/ragfs-mount         # macOS
```

### Permission denied errors

Ensure FUSE kernel module is loaded (Linux):

```bash
sudo modprobe fuse
```

On macOS, check System Settings > Privacy & Security for macFUSE extension permissions.

### File not found errors

Check your handler patterns match the requested paths. Enable debug mode to see FUSE operations.

## Performance Considerations

1. **Caching**: Implement caching in your handlers to avoid repeated data fetches
2. **Lazy Loading**: Only fetch data when files are actually opened
3. **Connection Pooling**: Reuse database/API connections across requests
4. **Timeouts**: Set appropriate context timeouts for handler operations

## Security Considerations

1. **Input Validation**: Validate all path parameters in handlers
2. **Access Control**: Implement authorization checks in handlers
3. **Secrets**: Never expose sensitive data without proper access controls
4. **Resource Limits**: Implement rate limiting and resource quotas

## Next Steps

- Review the example mount program in `examples/json/main.go`
- Check out `examples_test.go` for handler patterns
- Read `CLAUDE.md` for development guidelines
- See `README.md` for general library usage
