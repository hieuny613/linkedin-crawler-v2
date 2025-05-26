package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB represents the database connection
type DB struct {
	conn *sql.DB
}

// New creates a new database connection
func New(dbPath string) (*DB, error) {
	// Add connection parameters for better performance
	conn, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_timeout=5000", dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(1) // SQLite doesn't support multiple writers
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	db := &DB{conn: conn}

	// Initialize schema
	if err := db.InitSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// InitSchema creates all necessary tables (drops existing data)
func (db *DB) InitSchema() error {
	// Drop existing tables
	dropQueries := []string{
		`DROP TABLE IF EXISTS emails`,
		`DROP TABLE IF EXISTS tokens`,
		`DROP TABLE IF EXISTS accounts`,
	}

	for _, query := range dropQueries {
		if _, err := db.conn.Exec(query); err != nil {
			return fmt.Errorf("failed to drop table: %w", err)
		}
	}

	// Create new tables
	createQueries := []string{
		`CREATE TABLE emails (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			profile_user TEXT,
			profile_url TEXT,
			profile_location TEXT,
			profile_connections TEXT,
			retry_count INTEGER DEFAULT 0,
			last_error TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX idx_emails_status ON emails(status)`,
		`CREATE INDEX idx_emails_email ON emails(email)`,

		`CREATE TABLE tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token TEXT UNIQUE NOT NULL,
			is_valid BOOLEAN DEFAULT 1,
			failure_count INTEGER DEFAULT 0,
			last_used_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX idx_tokens_is_valid ON tokens(is_valid)`,

		`CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			is_used BOOLEAN DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX idx_accounts_is_used ON accounts(is_used)`,
	}

	for _, query := range createQueries {
		if _, err := db.conn.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// Begin starts a new transaction
func (db *DB) Begin() (*sql.Tx, error) {
	return db.conn.Begin()
}

// GetConn returns the underlying connection (for repositories)
func (db *DB) GetConn() *sql.DB {
	return db.conn
}
