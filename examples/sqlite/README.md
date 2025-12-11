# SQLite FUSE Mount Example

This example demonstrates how to mount a SQLite database as a filesystem using ragfs.

## Features

- **List tables**: Root directory shows all database tables as directories
- **List rows**: Each table directory shows all rows as files
- **Multiple formats**: Access each row in JSON, CSV, or TXT format
- **Primary key support**: Automatically detects and uses primary keys for row identification

## Building

```bash
# From the project root
go build -o sqlite-mount ./examples/sqlite

# Or from this directory
cd examples/sqlite
go build -o sqlite-mount .
```

## Creating Example Database

If you don't have a SQLite database handy, create one using the included script:

```bash
cd examples/sqlite
./create-example-db.sh
```

This creates `example.db` with sample tables:
- **users**: Sample user records (id, name, email, age, created_at)
- **posts**: Sample blog posts (id, user_id, title, content, published, created_at)
- **tags**: Sample tags (id, name)

## Usage

```bash
# Create mount point
mkdir -p /tmp/sqlite-mount

# Mount the database
./sqlite-mount -mount /tmp/sqlite-mount -db example.db

# In another terminal, explore the filesystem
ls /tmp/sqlite-mount/              # List all tables
ls /tmp/sqlite-mount/users/        # List all user records
cat /tmp/sqlite-mount/users/1.json # View user #1 as JSON
cat /tmp/sqlite-mount/users/1.csv  # View user #1 as CSV
cat /tmp/sqlite-mount/users/1.txt  # View user #1 as plain text

# Read different tables
cat /tmp/sqlite-mount/posts/1.json
cat /tmp/sqlite-mount/tags/2.csv

# Unmount (Ctrl-C in the mount terminal)
```

## Path Structure

The filesystem follows this structure:

```
/                           # Root - lists all tables
├── users/                  # Table directory
│   ├── 1.json             # Row with id=1 as JSON
│   ├── 1.csv              # Row with id=1 as CSV
│   ├── 1.txt              # Row with id=1 as plain text
│   ├── 2.json             # Row with id=2 as JSON
│   ├── 2.csv              # Row with id=2 as CSV
│   └── 2.txt              # Row with id=2 as plain text
├── posts/
│   ├── 1.json
│   ├── 1.csv
│   └── ...
└── tags/
    └── ...
```

## Format Examples

### JSON Format
```json
{
  "id": 1,
  "name": "Alice Johnson",
  "email": "alice@example.com",
  "age": 30,
  "created_at": "2025-12-10 12:00:00"
}
```

### CSV Format
```csv
id,name,email,age,created_at
1,Alice Johnson,alice@example.com,30,2025-12-10 12:00:00
```

### TXT Format
```
id: 1
name: Alice Johnson
email: alice@example.com
age: 30
created_at: 2025-12-10 12:00:00
```

## How It Works

The example maps SQLite database operations to filesystem paths:

1. **Root handler (`/`)**: Lists all tables in the database
2. **Table handler (`/{table}`)**: Lists all primary key values in a table, with entries for each format (json, csv, txt)
3. **Row handler (`/{table}/{id}.{ext}`)**: Fetches a specific row and formats it according to the extension

The implementation uses:
- SQLite `PRAGMA` commands to introspect table structure
- Dynamic SQL queries to fetch data
- Format conversion for JSON, CSV, and TXT output

## Notes

- If a table has no explicit primary key, the system uses SQLite's built-in `rowid`
- All data is read-only (no write operations)
- Large databases may take time to list all rows in a table
- Binary data (BLOBs) is not currently supported in a user-friendly way

## Use Cases

- Quick database inspection without SQL tools
- Integration with filesystem-based tools (grep, find, etc.)
- Exposing database data to LLM agents that understand filesystems
- Rapid prototyping and debugging
