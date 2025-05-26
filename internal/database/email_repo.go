package database

import (
	"database/sql"
	"fmt"
	"strings"

	"linkedin-crawler/internal/models"
)

// EmailStatus represents the status of an email
type EmailStatus string

const (
	EmailStatusPending         EmailStatus = "pending"
	EmailStatusSuccessWithData EmailStatus = "success_with_data"
	EmailStatusSuccessNoData   EmailStatus = "success_without_data"
	EmailStatusFailed          EmailStatus = "failed"
	EmailStatusPermanentFailed EmailStatus = "permanent_failed"
)

// EmailRepository handles email operations
type EmailRepository struct {
	db *sql.DB
}

// NewEmailRepository creates a new email repository
func NewEmailRepository(db *DB) *EmailRepository {
	return &EmailRepository{db: db.GetConn()}
}

// ImportEmails imports emails from a list (batch insert)
func (er *EmailRepository) ImportEmails(emails []string) error {
	if len(emails) == 0 {
		return nil
	}

	tx, err := er.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Prepare statement for batch insert
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO emails (email) VALUES (?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Batch insert
	for _, email := range emails {
		email = strings.TrimSpace(email)
		if email != "" && !strings.HasPrefix(email, "#") {
			if _, err := stmt.Exec(email); err != nil {
				return fmt.Errorf("failed to insert email %s: %w", email, err)
			}
		}
	}

	return tx.Commit()
}

// GetPendingEmails returns emails that need to be processed
func (er *EmailRepository) GetPendingEmails(limit int) ([]string, error) {
	query := `SELECT email FROM emails WHERE status = 'pending' ORDER BY id`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := er.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}

	return emails, rows.Err()
}

// GetEmailsByStatus returns emails with specific status
func (er *EmailRepository) GetEmailsByStatus(status EmailStatus) ([]string, error) {
	rows, err := er.db.Query(`SELECT email FROM emails WHERE status = ?`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}

	return emails, rows.Err()
}

// UpdateEmailStatus updates the status of an email
func (er *EmailRepository) UpdateEmailStatus(email string, status EmailStatus) error {
	_, err := er.db.Exec(`
		UPDATE emails 
		SET status = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE email = ?
	`, status, email)
	return err
}

// UpdateEmailWithProfile updates email with LinkedIn profile data
func (er *EmailRepository) UpdateEmailWithProfile(email string, profile models.ProfileData) error {
	_, err := er.db.Exec(`
		UPDATE emails 
		SET status = ?, 
			profile_user = ?, 
			profile_url = ?, 
			profile_location = ?, 
			profile_connections = ?,
			updated_at = CURRENT_TIMESTAMP 
		WHERE email = ?
	`, EmailStatusSuccessWithData, profile.User, profile.LinkedInURL,
		profile.Location, profile.ConnectionCount, email)
	return err
}

// IncrementRetryCount increments the retry count for an email
func (er *EmailRepository) IncrementRetryCount(email string, lastError string) error {
	_, err := er.db.Exec(`
		UPDATE emails 
		SET retry_count = retry_count + 1, 
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP 
		WHERE email = ?
	`, lastError, email)
	return err
}

// GetEmailStats returns statistics about emails
func (er *EmailRepository) GetEmailStats() (map[string]int, error) {
	rows, err := er.db.Query(`
		SELECT status, COUNT(*) as count 
		FROM emails 
		GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}

	// Add total count
	var total int
	err = er.db.QueryRow(`SELECT COUNT(*) FROM emails`).Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	return stats, nil
}

// GetRemainingEmails returns emails that still need processing
func (er *EmailRepository) GetRemainingEmails() ([]string, error) {
	rows, err := er.db.Query(`
		SELECT email FROM emails 
		WHERE status IN ('pending', 'failed') 
		ORDER BY retry_count ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}

	return emails, rows.Err()
}

// CountRemainingEmails returns the count of emails that need processing
func (er *EmailRepository) CountRemainingEmails() (int, error) {
	var count int
	err := er.db.QueryRow(`
		SELECT COUNT(*) FROM emails 
		WHERE status IN ('pending', 'failed')
	`).Scan(&count)
	return count, err
}
