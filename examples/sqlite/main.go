package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/brittcrawford/ragfs"
	fuseFS "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Parse command-line flags
	mountPoint := flag.String("mount", "", "Mount point directory (required)")
	dbPath := flag.String("db", "", "Path to SQLite database file (required)")
	flag.Parse()

	if *mountPoint == "" || *dbPath == "" {
		fmt.Println("Usage: sqlite-mount -mount <directory> -db <database>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Create ragfs filesystem
	fsys := ragfs.New()

	// Map root to list all tables
	fsys.Map("/", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tables, err := getTables(db)
		if err != nil {
			return nil, err
		}

		var entries []fs.DirEntry
		for _, table := range tables {
			entries = append(entries, &dirEntry{
				name:  table,
				isDir: true,
			})
		}
		return entries, nil
	})

	// Map /{table} to list all rows in that table
	fsys.Map("/{table}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tableName := params["table"]

		// Verify table exists
		tables, err := getTables(db)
		if err != nil {
			return nil, err
		}
		if !contains(tables, tableName) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Get primary key column
		pkCol, err := getPrimaryKey(db, tableName)
		if err != nil {
			return nil, err
		}

		// Get all primary key values
		rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s", pkCol, tableName))
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var entries []fs.DirEntry
		// Add special directories
		entries = append(entries,
			&dirEntry{name: "_metrics", isDir: true},
			&dirEntry{name: "_search", isDir: true},
		)

		for rows.Next() {
			var id interface{}
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			// Create entries for each format
			idStr := fmt.Sprintf("%v", id)
			entries = append(entries,
				&fileEntry{name: fmt.Sprintf("%s.json", idStr), content: nil},
				&fileEntry{name: fmt.Sprintf("%s.csv", idStr), content: nil},
				&fileEntry{name: fmt.Sprintf("%s.txt", idStr), content: nil},
			)
		}

		return entries, nil
	})

	// Map /{table}/{id}.{ext} to fetch a specific row
	fsys.Map("/{table}/{file}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tableName := params["table"]
		fileName := params["file"]

		// Skip special directories (they have their own handlers)
		if fileName == "_metrics" || fileName == "_search" {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Parse filename to extract id and extension
		ext := filepath.Ext(fileName)
		if ext == "" {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}
		ext = ext[1:] // Remove leading dot
		idStr := strings.TrimSuffix(fileName, "."+ext)

		// Validate extension
		if ext != "json" && ext != "csv" && ext != "txt" {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Get primary key column
		pkCol, err := getPrimaryKey(db, tableName)
		if err != nil {
			return nil, err
		}

		// Fetch the row
		query := fmt.Sprintf("SELECT * FROM %s WHERE %s = ?", tableName, pkCol)
		row := db.QueryRowContext(ctx, query, idStr)

		// Get column names
		columns, err := getColumns(db, tableName)
		if err != nil {
			return nil, err
		}

		// Scan the row into a map
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := row.Scan(valuePtrs...); err != nil {
			if err == sql.ErrNoRows {
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
			return nil, err
		}

		// Build the row data map
		rowData := make(map[string]interface{})
		for i, col := range columns {
			rowData[col] = values[i]
		}

		// Format based on extension
		var content []byte
		switch ext {
		case "json":
			content, err = json.MarshalIndent(rowData, "", "  ")
		case "csv":
			content, err = formatCSV(columns, values)
		case "txt":
			content = []byte(formatTXT(rowData))
		}

		if err != nil {
			return nil, err
		}

		return []fs.DirEntry{
			&fileEntry{
				name:    fileName,
				content: content,
			},
		}, nil
	})

	// Map /{table}/_metrics to list available metrics
	fsys.Map("/{table}/_metrics", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tableName := params["table"]

		// Verify table exists
		tables, err := getTables(db)
		if err != nil {
			return nil, err
		}
		if !contains(tables, tableName) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Return available metrics
		return []fs.DirEntry{
			&fileEntry{name: "row_count", content: nil},
		}, nil
	})

	// Map /{table}/_metrics/{metric} to fetch metric value
	fsys.Map("/{table}/_metrics/{metric}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tableName := params["table"]
		metric := params["metric"]

		// Verify table exists
		tables, err := getTables(db)
		if err != nil {
			return nil, err
		}
		if !contains(tables, tableName) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		var content []byte
		switch metric {
		case "row_count":
			var count int
			err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
			if err != nil {
				return nil, err
			}
			content = []byte(fmt.Sprintf("%d\n", count))
		default:
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		return []fs.DirEntry{
			&fileEntry{
				name:    metric,
				content: content,
			},
		}, nil
	})

	// Map /{table}/_search to list search directory (currently no listing needed)
	fsys.Map("/{table}/_search", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tableName := params["table"]

		// Verify table exists
		tables, err := getTables(db)
		if err != nil {
			return nil, err
		}
		if !contains(tables, tableName) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Return empty directory (searches are accessed directly via file paths)
		return []fs.DirEntry{}, nil
	})

	// Map /{table}/_search/{search_file} to perform searches
	// Format: {column}.{value}.{ext}
	fsys.Map("/{table}/_search/{search_file}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		tableName := params["table"]
		searchFile := params["search_file"]

		// Parse search file: column.value.ext
		parts := strings.SplitN(searchFile, ".", 3)
		if len(parts) != 3 {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}
		column := parts[0]
		value := parts[1]
		ext := parts[2]

		// Validate extension
		if ext != "json" && ext != "csv" && ext != "txt" {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Verify table exists
		tables, err := getTables(db)
		if err != nil {
			return nil, err
		}
		if !contains(tables, tableName) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Get column names to verify column exists
		columns, err := getColumns(db, tableName)
		if err != nil {
			return nil, err
		}
		if !contains(columns, column) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Execute search query
		query := fmt.Sprintf("SELECT * FROM %s WHERE %s = ?", tableName, column)
		rows, err := db.QueryContext(ctx, query, value)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		// Collect all matching rows
		var allRows []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, err
			}

			rowData := make(map[string]interface{})
			for i, col := range columns {
				rowData[col] = values[i]
			}
			allRows = append(allRows, rowData)
		}

		// Format based on extension
		var content []byte
		switch ext {
		case "json":
			content, err = json.MarshalIndent(allRows, "", "  ")
		case "csv":
			content, err = formatMultiRowCSV(columns, allRows)
		case "txt":
			var buf strings.Builder
			for i, row := range allRows {
				if i > 0 {
					buf.WriteString("\n---\n\n")
				}
				buf.WriteString(formatTXT(row))
			}
			content = []byte(buf.String())
		}

		if err != nil {
			return nil, err
		}

		return []fs.DirEntry{
			&fileEntry{
				name:    searchFile,
				content: content,
			},
		}, nil
	})

	// Create FUSE root
	root := ragfs.NewFUSERoot(fsys)

	log.Printf("About to mount at %s", *mountPoint)

	// Mount the filesystem
	server, err := fuseFS.Mount(*mountPoint, root, &fuseFS.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			FsName:     "ragfs-sqlite",
			Name:       "ragfs-sqlite",
			AllowOther: false,
		},
	})
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}

	log.Printf("Mounted ragfs-sqlite at %s", *mountPoint)
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

// getTables returns all table names in the database
func getTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, nil
}

// getPrimaryKey returns the primary key column name for a table
func getPrimaryKey(db *sql.DB, tableName string) (string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return "", err
		}
		if pk > 0 {
			return name, nil
		}
	}

	// If no primary key found, use rowid
	return "rowid", nil
}

// getColumns returns all column names for a table
func getColumns(db *sql.DB, tableName string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, nil
}

// contains checks if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// formatCSV formats a row as CSV
func formatCSV(columns []string, values []interface{}) ([]byte, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	// Write header
	if err := writer.Write(columns); err != nil {
		return nil, err
	}

	// Write values
	strValues := make([]string, len(values))
	for i, v := range values {
		if v == nil {
			strValues[i] = ""
		} else {
			strValues[i] = fmt.Sprintf("%v", v)
		}
	}
	if err := writer.Write(strValues); err != nil {
		return nil, err
	}

	writer.Flush()
	return []byte(buf.String()), writer.Error()
}

// formatMultiRowCSV formats multiple rows as CSV
func formatMultiRowCSV(columns []string, allRows []map[string]interface{}) ([]byte, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	// Write header
	if err := writer.Write(columns); err != nil {
		return nil, err
	}

	// Write all rows
	for _, rowData := range allRows {
		strValues := make([]string, len(columns))
		for i, col := range columns {
			if rowData[col] == nil {
				strValues[i] = ""
			} else {
				strValues[i] = fmt.Sprintf("%v", rowData[col])
			}
		}
		if err := writer.Write(strValues); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return []byte(buf.String()), writer.Error()
}

// formatTXT formats a row as plain text
func formatTXT(rowData map[string]interface{}) string {
	var buf strings.Builder
	for key, value := range rowData {
		buf.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}
	return buf.String()
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
