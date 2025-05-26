// internal/storage/db_storage.go
package storage

import (
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	"linkedin-crawler/internal/database"
	"linkedin-crawler/internal/models"
)

// DBStorage manages all storage operations using SQLite
type DBStorage struct {
	DB          *database.DB
	EmailRepo   *database.EmailRepository
	TokenRepo   *database.TokenRepository
	AccountRepo *database.AccountRepository
	mutex       sync.Mutex
}

// NewDBStorage creates a new database storage
func NewDBStorage(dbPath string) (*DBStorage, error) {
	db, err := database.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return &DBStorage{
		DB:          db,
		EmailRepo:   database.NewEmailRepository(db),
		TokenRepo:   database.NewTokenRepository(db),
		AccountRepo: database.NewAccountRepository(db),
	}, nil
}

// Close closes the database connection
func (ds *DBStorage) Close() error {
	return ds.DB.Close()
}

// ImportEmailsFromFile imports emails from file into database
func (ds *DBStorage) ImportEmailsFromFile(filePath string) error {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read emails file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var emails []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		email := line

		// Handle CSV format
		if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			email = strings.TrimSpace(parts[1])
		}

		if email != "" && !strings.HasPrefix(email, "#") {
			emails = append(emails, email)
		}
	}

	return ds.EmailRepo.ImportEmails(emails)
}

// ImportAccountsFromFile imports accounts from file into database
func (ds *DBStorage) ImportAccountsFromFile(filePath string) error {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read accounts file: %w", err)
	}

	accounts, err := ds.AccountRepo.LoadAccountsFromText(string(content))
	if err != nil {
		return err
	}

	return ds.AccountRepo.ImportAccounts(accounts)
}

// ImportTokensFromFile imports tokens from file into database
func (ds *DBStorage) ImportTokensFromFile(filePath string) error {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		// No existing tokens file is OK
		return nil
	}

	lines := strings.Split(string(content), "\n")
	var tokens []string

	for _, line := range lines {
		token := strings.TrimSpace(line)
		if token != "" {
			token = strings.TrimPrefix(token, "Bearer ")
			tokens = append(tokens, token)
		}
	}

	return ds.TokenRepo.AddTokens(tokens)
}

// Updated EmailStorage to use database
type EmailStorage struct {
	dbStorage *DBStorage
}

// NewEmailStorage creates a new EmailStorage that uses database
func NewEmailStorage() *EmailStorage {
	// This will be initialized in the main orchestrator
	return &EmailStorage{}
}

// SetDBStorage sets the database storage
func (es *EmailStorage) SetDBStorage(ds *DBStorage) {
	es.dbStorage = ds
}

// LoadEmailsFromFile returns pending emails from database
func (es *EmailStorage) LoadEmailsFromFile(filePath string) ([]string, error) {
	if es.dbStorage == nil {
		return nil, fmt.Errorf("database storage not initialized")
	}
	return es.dbStorage.EmailRepo.GetRemainingEmails()
}

// WriteEmailsToFile is now a no-op as we use database
func (es *EmailStorage) WriteEmailsToFile(filePath string, emails []string) error {
	// No longer needed - database handles persistence
	return nil
}

// RemoveEmailFromFile updates email status in database
func (es *EmailStorage) RemoveEmailFromFile(filePath string, emailToRemove string) error {
	if es.dbStorage == nil {
		return fmt.Errorf("database storage not initialized")
	}
	// This is called when email is successfully processed
	// Status will be updated by the caller with specific status
	return nil
}

// Updated TokenStorage to use database
type TokenStorage struct {
	dbStorage *DBStorage
}

// NewTokenStorage creates a new TokenStorage that uses database
func NewTokenStorage() *TokenStorage {
	return &TokenStorage{}
}

// SetDBStorage sets the database storage
func (ts *TokenStorage) SetDBStorage(ds *DBStorage) {
	ts.dbStorage = ds
}

// LoadTokensFromFile returns valid tokens from database
func (ts *TokenStorage) LoadTokensFromFile(filePath string) ([]string, error) {
	if ts.dbStorage == nil {
		return nil, fmt.Errorf("database storage not initialized")
	}
	return ts.dbStorage.TokenRepo.GetValidTokens()
}

// SaveTokensToFile adds tokens to database
func (ts *TokenStorage) SaveTokensToFile(filePath string, tokens []string) error {
	if ts.dbStorage == nil {
		return fmt.Errorf("database storage not initialized")
	}
	return ts.dbStorage.TokenRepo.AddTokens(tokens)
}

// RemoveTokenFromFile invalidates token in database
func (ts *TokenStorage) RemoveTokenFromFile(filePath string, tokenToRemove string) error {
	if ts.dbStorage == nil {
		return fmt.Errorf("database storage not initialized")
	}
	return ts.dbStorage.TokenRepo.InvalidateToken(tokenToRemove)
}

// Updated AccountStorage to use database
type AccountStorage struct {
	dbStorage *DBStorage
}

// NewAccountStorage creates a new AccountStorage that uses database
func NewAccountStorage() *AccountStorage {
	return &AccountStorage{}
}

// SetDBStorage sets the database storage
func (as *AccountStorage) SetDBStorage(ds *DBStorage) {
	as.dbStorage = ds
}

// LoadAccounts returns unused accounts from database
func (as *AccountStorage) LoadAccounts(filename string) ([]models.Account, error) {
	if as.dbStorage == nil {
		return nil, fmt.Errorf("database storage not initialized")
	}
	// Get all unused accounts
	return as.dbStorage.AccountRepo.GetUnusedAccounts(0)
}

// RemoveAccountFromFile marks account as used in database
func (as *AccountStorage) RemoveAccountFromFile(filePath string, acc models.Account) error {
	if as.dbStorage == nil {
		return fmt.Errorf("database storage not initialized")
	}
	return as.dbStorage.AccountRepo.MarkAccountAsUsed(acc.Email)
}

// Legacy function wrappers - now use database
var (
	globalDBStorage      *DBStorage
	globalEmailStorage   = NewEmailStorage()
	globalTokenStorage   = NewTokenStorage()
	globalAccountStorage = NewAccountStorage()
)

// InitializeDatabase initializes the global database storage
func InitializeDatabase(dbPath string) error {
	ds, err := NewDBStorage(dbPath)
	if err != nil {
		return err
	}

	globalDBStorage = ds
	globalEmailStorage.SetDBStorage(ds)
	globalTokenStorage.SetDBStorage(ds)
	globalAccountStorage.SetDBStorage(ds)

	return nil
}

// CloseDatabase closes the global database
func CloseDatabase() error {
	if globalDBStorage != nil {
		return globalDBStorage.Close()
	}
	return nil
}

// GetDBStorage returns the global database storage
func GetDBStorage() *DBStorage {
	return globalDBStorage
}
