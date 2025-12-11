# JSON FUSE Mount Example

This example demonstrates how to mount a JSON configuration file as a filesystem using ragfs.

## Features

- **Navigate JSON as directories**: JSON objects become directories
- **Access values as files**: Primitive values (strings, numbers, booleans) become files
- **Array support**: JSON arrays are presented as numbered directories (0, 1, 2, ...)
- **Automatic type detection**: The filesystem automatically determines whether a path should be a file or directory

## Building

```bash
# From the project root
go build -o ragfs-mount ./examples/json

# Or from this directory
cd examples/json
go build -o ragfs-mount .
```

## Usage

First, create a JSON configuration file (or use the provided `config.json`):

```json
{
  "app": {
    "name": "MyApp",
    "version": "1.0.0",
    "port": 8080
  },
  "database": {
    "host": "localhost",
    "port": 5432,
    "name": "mydb"
  },
  "features": ["auth", "logging", "metrics"]
}
```

Then mount it:

```bash
# Create mount point
mkdir -p /tmp/ragfs-mount

# Mount the JSON file
./ragfs-mount -mount /tmp/ragfs-mount -config config.json

# In another terminal, explore the filesystem
ls /tmp/ragfs-mount/              # Lists: app, database, features
ls /tmp/ragfs-mount/app/          # Lists: name, port, version
cat /tmp/ragfs-mount/app/name     # Output: MyApp
cat /tmp/ragfs-mount/app/port     # Output: 8080

# Navigate arrays
ls /tmp/ragfs-mount/features/     # Lists: 0, 1, 2
cat /tmp/ragfs-mount/features/0   # Output: auth
cat /tmp/ragfs-mount/features/1   # Output: logging

# Unmount (Ctrl-C in the mount terminal)
```

## Path Structure

The filesystem structure mirrors the JSON structure:

```
/                                  # Root - top-level JSON object
├── app/                          # JSON object -> directory
│   ├── name                      # String value -> file
│   ├── port                      # Number value -> file
│   └── version                   # String value -> file
├── database/                     # JSON object -> directory
│   ├── host                      # String value -> file
│   ├── port                      # Number value -> file
│   └── name                      # String value -> file
└── features/                     # JSON array -> directory
    ├── 0                         # Array element -> file
    ├── 1                         # Array element -> file
    └── 2                         # Array element -> file
```

## How It Works

The example maps JSON paths to filesystem paths:

1. **Root handler (`/*`)**: Handles all paths by parsing the JSON structure
2. **Path parsing**: Converts filesystem paths like `/app/name` to JSON path `["app", "name"]`
3. **Type detection**:
   - JSON objects/arrays → Directories with entries for each key/index
   - Primitive values (string, number, boolean) → Files with the value as content
4. **Navigation**: Each directory entry is marked as either a file or directory based on its JSON type

## Command-Line Options

```
-mount <directory>    Mount point directory (required)
-config <file>        JSON configuration file to expose (default: "config.json")
```

## Example JSON File

The repository includes a sample `config.json` in the project root:

```json
{
  "app": {
    "name": "MyApp",
    "version": "1.0.0",
    "port": 8080
  },
  "database": {
    "host": "localhost",
    "port": 5432,
    "name": "mydb",
    "pool": {
      "min": 5,
      "max": 20
    }
  },
  "features": ["auth", "logging", "metrics"]
}
```

## Use Cases

- **Configuration inspection**: View complex config files using familiar file tools
- **LLM agents**: Allow AI agents to explore configuration using filesystem commands
- **Debugging**: Quick access to nested configuration values
- **Shell scripts**: Read config values in shell scripts using `cat`
- **Integration testing**: Test applications by mounting different configs

## Notes

- All data is read-only (no write operations)
- The JSON file is loaded once at startup
- Changes to the JSON file require remounting to be visible
- Nested objects and arrays of any depth are supported
