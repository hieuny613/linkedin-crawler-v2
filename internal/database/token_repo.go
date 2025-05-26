package database

import (
	"database/sql"
	"fmt"
)

// TokenRepository handles token operations
type TokenRepository struct {
	db *sql.DB
}

// NewTokenRepository creates a new token repository
func NewTokenRepository(db *DB) *TokenRepository {
	return &TokenRepository{db: db.GetConn()}
}

// AddToken adds a new token
func (tr *TokenRepository) AddToken(token string) error {
	_, err := tr.db.Exec(`
		INSERT OR IGNORE INTO tokens (token) VALUES (?)
	`, token)
	return err
}

// AddTokens adds multiple tokens (batch insert)
func (tr *TokenRepository) AddTokens(tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}

	tx, err := tr.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO tokens (token) VALUES (?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, token := range tokens {
		if _, err := stmt.Exec(token); err != nil {
			return fmt.Errorf("failed to insert token: %w", err)
		}
	}

	return tx.Commit()
}

// GetValidTokens returns all valid tokens
func (tr *TokenRepository) GetValidTokens() ([]string, error) {
	rows, err := tr.db.Query(`
		SELECT token FROM tokens 
		WHERE is_valid = 1 
		ORDER BY COALESCE(last_used_at, created_at) ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// MarkTokenAsUsed updates the last used timestamp
func (tr *TokenRepository) MarkTokenAsUsed(token string) error {
	_, err := tr.db.Exec(`
		UPDATE tokens 
		SET last_used_at = CURRENT_TIMESTAMP, 
			updated_at = CURRENT_TIMESTAMP 
		WHERE token = ?
	`, token)
	return err
}

// InvalidateToken marks a token as invalid
func (tr *TokenRepository) InvalidateToken(token string) error {
	_, err := tr.db.Exec(`
		UPDATE tokens 
		SET is_valid = 0, 
			updated_at = CURRENT_TIMESTAMP 
		WHERE token = ?
	`, token)
	return err
}

// IncrementTokenFailure increments failure count
func (tr *TokenRepository) IncrementTokenFailure(token string) error {
	_, err := tr.db.Exec(`
		UPDATE tokens 
		SET failure_count = failure_count + 1, 
			updated_at = CURRENT_TIMESTAMP 
		WHERE token = ?
	`, token)
	return err
}

// GetValidTokenCount returns the count of valid tokens
func (tr *TokenRepository) GetValidTokenCount() (int, error) {
	var count int
	err := tr.db.QueryRow(`
		SELECT COUNT(*) FROM tokens WHERE is_valid = 1
	`).Scan(&count)
	return count, err
}

// ImportTokensFromFile imports tokens from existing file
func (tr *TokenRepository) ImportTokensFromFile(tokens []string) error {
	return tr.AddTokens(tokens)
}
