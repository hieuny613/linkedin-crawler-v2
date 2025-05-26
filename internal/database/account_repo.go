package database

import (
	"database/sql"
	"fmt"
	"strings"

	"linkedin-crawler/internal/models"
)

// AccountRepository handles account operations
type AccountRepository struct {
	db *sql.DB
}

// NewAccountRepository creates a new account repository
func NewAccountRepository(db *DB) *AccountRepository {
	return &AccountRepository{db: db.GetConn()}
}

// ImportAccounts imports accounts from a list
func (ar *AccountRepository) ImportAccounts(accounts []models.Account) error {
	if len(accounts) == 0 {
		return nil
	}

	tx, err := ar.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO accounts (email, password) VALUES (?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, account := range accounts {
		if _, err := stmt.Exec(account.Email, account.Password); err != nil {
			return fmt.Errorf("failed to insert account %s: %w", account.Email, err)
		}
	}

	return tx.Commit()
}

// GetUnusedAccounts returns accounts that haven't been used yet
func (ar *AccountRepository) GetUnusedAccounts(limit int) ([]models.Account, error) {
	query := `SELECT email, password FROM accounts WHERE is_used = 0 ORDER BY id`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := ar.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var account models.Account
		if err := rows.Scan(&account.Email, &account.Password); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	return accounts, rows.Err()
}

// MarkAccountAsUsed marks an account as used
func (ar *AccountRepository) MarkAccountAsUsed(email string) error {
	_, err := ar.db.Exec(`
		UPDATE accounts 
		SET is_used = 1, 
			updated_at = CURRENT_TIMESTAMP 
		WHERE email = ?
	`, email)
	return err
}

// GetUnusedAccountCount returns the count of unused accounts
func (ar *AccountRepository) GetUnusedAccountCount() (int, error) {
	var count int
	err := ar.db.QueryRow(`
		SELECT COUNT(*) FROM accounts WHERE is_used = 0
	`).Scan(&count)
	return count, err
}

// LoadAccountsFromText parses accounts from text content
func (ar *AccountRepository) LoadAccountsFromText(content string) ([]models.Account, error) {
	var accounts []models.Account
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])

		if email != "" && password != "" {
			accounts = append(accounts, models.Account{
				Email:    email,
				Password: password,
			})
		}
	}

	return accounts, nil
}
