#!/bin/bash
# Create an example SQLite database for testing ragfs-sqlite

DB_FILE="example.db"

# Remove existing database if it exists
rm -f "$DB_FILE"

# Create database with sample tables
sqlite3 "$DB_FILE" <<EOF
-- Create users table
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    age INTEGER,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Insert sample users
INSERT INTO users (name, email, age) VALUES
    ('Alice Johnson', 'alice@example.com', 30),
    ('Bob Smith', 'bob@example.com', 25),
    ('Charlie Brown', 'charlie@example.com', 35),
    ('Diana Prince', 'diana@example.com', 28);

-- Create posts table
CREATE TABLE posts (
    id INTEGER PRIMARY KEY,
    user_id INTEGER,
    title TEXT NOT NULL,
    content TEXT,
    published INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Insert sample posts
INSERT INTO posts (user_id, title, content, published) VALUES
    (1, 'First Post', 'This is my first post!', 1),
    (1, 'Learning Go', 'Go is an amazing language.', 1),
    (2, 'Database Tips', 'Here are some SQLite tips...', 1),
    (3, 'Hello World', 'Just saying hello to everyone.', 0),
    (4, 'FUSE Filesystems', 'FUSE is really powerful.', 1);

-- Create tags table
CREATE TABLE tags (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

-- Insert sample tags
INSERT INTO tags (name) VALUES
    ('technology'),
    ('programming'),
    ('database'),
    ('tutorial');

EOF

echo "Example database created: $DB_FILE"
echo ""
echo "Tables created:"
sqlite3 "$DB_FILE" "SELECT name FROM sqlite_master WHERE type='table';"
echo ""
echo "Sample data:"
echo "  - users: 4 records"
echo "  - posts: 5 records"
echo "  - tags: 4 records"
