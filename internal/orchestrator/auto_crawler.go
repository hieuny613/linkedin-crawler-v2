package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"linkedin-crawler/internal/database"
	"linkedin-crawler/internal/models"
	"linkedin-crawler/internal/storage"
	"linkedin-crawler/internal/utils"
)

// AutoCrawler orchestrates the LinkedIn crawling process
type AutoCrawler struct {
	config            models.Config
	accounts          []models.Account
	usedAccountIndex  int
	crawler           *models.LinkedInCrawler
	crawlerMutex      sync.RWMutex
	outputFile        string
	totalEmails       []string
	processedEmails   int
	shutdownRequested int32

	logFile      *os.File
	logWriter    *bufio.Writer
	logChan      chan string
	logWaitGroup sync.WaitGroup

	// Database storage
	dbStorage *storage.DBStorage

	// Email tracking maps (kept for compatibility but data is in DB)
	successEmailsWithData    map[string]struct{}
	successEmailsWithoutData map[string]struct{}
	failedEmails             map[string]struct{}
	permanentFailed          map[string]struct{}
	emailsMutex              sync.Mutex

	// File operation mutex
	fileOpMutex sync.Mutex

	// Storage services
	emailStorage   *storage.EmailStorage
	tokenStorage   *storage.TokenStorage
	accountStorage *storage.AccountStorage

	// Processing services
	batchProcessor *BatchProcessor
	retryHandler   *RetryHandler
	stateManager   *StateManager
}

// New creates a new AutoCrawler instance with SQLite support
func New(config models.Config) (*AutoCrawler, error) {
	outputFile := "hit.txt"

	// Initialize database
	dbPath := "crawler.db"
	if err := storage.InitializeDatabase(dbPath); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	dbStorage := storage.GetDBStorage()

	// Import data from files into database
	fmt.Println("📥 Importing data into database...")

	// Import accounts
	if err := dbStorage.ImportAccountsFromFile(config.AccountsFilePath); err != nil {
		return nil, fmt.Errorf("failed to import accounts: %w", err)
	}

	// Import emails
	if err := dbStorage.ImportEmailsFromFile(config.EmailsFilePath); err != nil {
		return nil, fmt.Errorf("failed to import emails: %w", err)
	}

	// Import existing tokens if any
	if err := dbStorage.ImportTokensFromFile(config.TokensFilePath); err != nil {
		fmt.Printf("⚠️ No existing tokens to import: %v\n", err)
	}

	// Get stats from database
	stats, err := dbStorage.EmailRepo.GetEmailStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get email stats: %w", err)
	}

	// Initialize storage services
	emailStorage := storage.NewEmailStorage()
	emailStorage.SetDBStorage(dbStorage)
	tokenStorage := storage.NewTokenStorage()
	tokenStorage.SetDBStorage(dbStorage)
	accountStorage := storage.NewAccountStorage()
	accountStorage.SetDBStorage(dbStorage)

	// Load data from database
	accounts, err := accountStorage.LoadAccounts(config.AccountsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load accounts: %w", err)
	}

	emails, err := dbStorage.EmailRepo.GetPendingEmails(0)
	if err != nil {
		return nil, fmt.Errorf("failed to load emails: %w", err)
	}
	// Setup logging
	logFile, err := os.OpenFile("crawler.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	ac := &AutoCrawler{
		config:           config,
		accounts:         accounts,
		usedAccountIndex: 0,
		outputFile:       outputFile,
		totalEmails:      emails,
		processedEmails:  0,
		logFile:          logFile,
		logWriter:        bufio.NewWriter(logFile),
		logChan:          make(chan string, 1000),
		dbStorage:        dbStorage,

		// Initialize email tracking maps
		successEmailsWithData:    make(map[string]struct{}),
		successEmailsWithoutData: make(map[string]struct{}),
		failedEmails:             make(map[string]struct{}),
		permanentFailed:          make(map[string]struct{}),

		// Initialize storage services
		emailStorage:   emailStorage,
		tokenStorage:   tokenStorage,
		accountStorage: accountStorage,
	}

	// Initialize processing services
	ac.batchProcessor = NewBatchProcessor(ac)
	ac.retryHandler = NewRetryHandler(ac)
	ac.stateManager = NewStateManager(ac)

	// Start logging goroutine
	ac.logWaitGroup.Add(1)
	go func() {
		defer ac.logWaitGroup.Done()
		for line := range ac.logChan {
			_, err := ac.logWriter.WriteString(line + "\n")
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ Lỗi ghi log: %v\n", err)
			}
		}
		ac.logWriter.Flush()
		ac.logFile.Close()
	}()

	// Setup signal handling
	utils.SetupSignalHandling(&ac.shutdownRequested, ac.stateManager.SaveStateOnShutdown, config.SleepDuration)

	// Print import stats
	fmt.Printf("✅ Database initialized successfully:\n")
	fmt.Printf("   📧 Total emails: %d\n", stats["total"])
	fmt.Printf("   📂 Total accounts: %d\n", len(accounts))
	fmt.Printf("   🔑 Total tokens: %d\n", stats["tokens"])
	fmt.Println(strings.Repeat("=", 80))

	return ac, nil
}

// UpdateEmailStatus updates email status in database
func (ac *AutoCrawler) UpdateEmailStatus(email string, status database.EmailStatus) error {
	return ac.dbStorage.EmailRepo.UpdateEmailStatus(email, status)
}

// UpdateEmailWithProfile updates email with profile data in database
func (ac *AutoCrawler) UpdateEmailWithProfile(email string, profile models.ProfileData) error {
	return ac.dbStorage.EmailRepo.UpdateEmailWithProfile(email, profile)
}

func (ac *AutoCrawler) LogLine(line string) {
	select {
	case ac.logChan <- line:
	default:
		fmt.Fprintf(os.Stderr, "⚠️ Log channel đầy, bỏ qua log: %s\n", line)
	}
}

// Run starts the crawling process
func (ac *AutoCrawler) Run() error {
	defer func() {
		// Close database
		storage.CloseDatabase()

		if atomic.LoadInt32(&ac.shutdownRequested) == 0 {
			fmt.Printf("💤 Sleep %v trước khi thoát...\n", ac.config.SleepDuration)
			time.Sleep(ac.config.SleepDuration)
		}
	}()

	fmt.Printf("🚀 Bắt đầu Auto LinkedIn Crawler với SQLite\n")

	// Get stats from database
	stats, _ := ac.dbStorage.EmailRepo.GetEmailStats()
	accountCount, _ := ac.dbStorage.AccountRepo.GetUnusedAccountCount()
	tokenCount, _ := ac.dbStorage.TokenRepo.GetValidTokenCount()

	fmt.Printf("📊 Database stats:\n")
	fmt.Printf("   📧 Total emails: %d\n", stats["total"])
	fmt.Printf("   ⏳ Pending: %d\n", stats["pending"])
	fmt.Printf("   📂 Unused accounts: %d\n", accountCount)
	fmt.Printf("   🔑 Valid tokens: %d\n", tokenCount)
	fmt.Println(strings.Repeat("=", 80))

	// Phase 1 - Xử lý tất cả emails
	if err := ac.batchProcessor.ProcessAllEmails(); err != nil {
		return err
	}

	// Phase 2 - Retry emails thất bại
	if err := ac.retryHandler.RetryFailedEmails(); err != nil {
		fmt.Printf("⚠️ Lỗi khi retry emails bị thất bại: %v\n", err)
	}

	close(ac.logChan)
	ac.logWaitGroup.Wait()

	// Print final results
	ac.printFinalResults()

	return nil
}

// printFinalResults prints the final crawling results from database
func (ac *AutoCrawler) printFinalResults() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🎉 HOÀN THÀNH AUTO LINKEDIN CRAWLER!")
	fmt.Println(strings.Repeat("=", 80))

	// Get final stats from database
	stats, err := ac.dbStorage.EmailRepo.GetEmailStats()
	if err != nil {
		fmt.Printf("⚠️ Lỗi khi lấy thống kê: %v\n", err)
		return
	}

	total := stats["total"]
	successWithData := stats[string(database.EmailStatusSuccessWithData)]
	successNoData := stats[string(database.EmailStatusSuccessNoData)]
	failed := stats[string(database.EmailStatusFailed)]
	permanentFailed := stats[string(database.EmailStatusPermanentFailed)]
	pending := stats[string(database.EmailStatusPending)]

	totalProcessed := successWithData + successNoData + failed + permanentFailed
	totalSuccess := successWithData + successNoData

	// Calculate percentages
	successPercent := float64(totalSuccess) * 100 / float64(total)
	dataPercent := 0.0
	if totalSuccess > 0 {
		dataPercent = float64(successWithData) * 100 / float64(totalSuccess)
	}

	fmt.Printf("📈 TỔNG KẾT CUỐI CÙNG:\n")
	fmt.Printf("   📊 Tổng emails xử lý:     %d/%d (%.1f%%)\n", totalProcessed, total, float64(totalProcessed)*100/float64(total))
	fmt.Printf("   ✅ Thành công:           %d/%d (%.1f%%)\n", totalSuccess, total, successPercent)
	fmt.Printf("   \n")
	fmt.Printf("   🎯 CÓ THÔNG TIN LINKEDIN: %d emails (%.1f%% trong thành công)\n", successWithData, dataPercent)
	fmt.Printf("   📭 KHÔNG CÓ THÔNG TIN:   %d emails (%.1f%% trong thành công)\n", successNoData, 100-dataPercent)
	fmt.Printf("   \n")
	fmt.Printf("   ⏳ Chưa xử lý:           %d emails\n", pending)
	fmt.Printf("   ❌ Cần retry:            %d emails\n", failed)
	fmt.Printf("   💀 Lỗi vĩnh viễn:        %d emails\n", permanentFailed)

	if successWithData > 0 {
		fmt.Printf("\n🎉 TÌM THẤY %d PROFILES LINKEDIN - Kết quả trong file: %s\n", successWithData, ac.outputFile)
	} else {
		fmt.Printf("\n😔 Không tìm thấy profile LinkedIn nào\n")
	}

	fmt.Println(strings.Repeat("=", 80))
}

// PrintCurrentStats prints current processing statistics from database
func (ac *AutoCrawler) PrintCurrentStats() {
	stats, err := ac.dbStorage.EmailRepo.GetEmailStats()
	if err != nil {
		return
	}

	withData := stats[string(database.EmailStatusSuccessWithData)]
	withoutData := stats[string(database.EmailStatusSuccessNoData)]
	failed := stats[string(database.EmailStatusFailed)]
	permanent := stats[string(database.EmailStatusPermanentFailed)]
	total := stats["total"]

	processed := withData + withoutData + permanent
	fmt.Printf("📊 Stats: ✅%d 📭%d ❌%d 💀%d | Progress: %d/%d (%.1f%%)\n",
		withData, withoutData, failed, permanent, processed, total, float64(processed)*100/float64(total))
}

// GetDBStorage returns the database storage
func (ac *AutoCrawler) GetDBStorage() *storage.DBStorage {
	return ac.dbStorage
}

// Getter methods for service access
func (ac *AutoCrawler) GetConfig() models.Config {
	return ac.config
}

func (ac *AutoCrawler) GetTotalEmails() []string {
	return ac.totalEmails
}

func (ac *AutoCrawler) GetAccounts() []models.Account {
	return ac.accounts
}

func (ac *AutoCrawler) GetUsedAccountIndex() int {
	return ac.usedAccountIndex
}

func (ac *AutoCrawler) SetUsedAccountIndex(index int) {
	ac.usedAccountIndex = index
}

func (ac *AutoCrawler) GetOutputFile() string {
	return ac.outputFile
}

func (ac *AutoCrawler) GetStorageServices() (*storage.EmailStorage, *storage.TokenStorage, *storage.AccountStorage) {
	return ac.emailStorage, ac.tokenStorage, ac.accountStorage
}

func (ac *AutoCrawler) GetEmailMaps() (map[string]struct{}, map[string]struct{}, map[string]struct{}, map[string]struct{}) {
	ac.emailsMutex.Lock()
	defer ac.emailsMutex.Unlock()

	// Return copies to prevent external modification
	withData := make(map[string]struct{})
	withoutData := make(map[string]struct{})
	failed := make(map[string]struct{})
	permanent := make(map[string]struct{})

	for k, v := range ac.successEmailsWithData {
		withData[k] = v
	}
	for k, v := range ac.successEmailsWithoutData {
		withoutData[k] = v
	}
	for k, v := range ac.failedEmails {
		failed[k] = v
	}
	for k, v := range ac.permanentFailed {
		permanent[k] = v
	}

	return withData, withoutData, failed, permanent
}

func (ac *AutoCrawler) UpdateEmailMaps(withData, withoutData, failed, permanent map[string]struct{}) {
	ac.emailsMutex.Lock()
	defer ac.emailsMutex.Unlock()

	ac.successEmailsWithData = withData
	ac.successEmailsWithoutData = withoutData
	ac.failedEmails = failed
	ac.permanentFailed = permanent
}

func (ac *AutoCrawler) AddEmailToMap(email string, mapType string) {
	ac.emailsMutex.Lock()
	defer ac.emailsMutex.Unlock()

	switch mapType {
	case "withData":
		ac.successEmailsWithData[email] = struct{}{}
	case "withoutData":
		ac.successEmailsWithoutData[email] = struct{}{}
	case "failed":
		ac.failedEmails[email] = struct{}{}
	case "permanent":
		ac.permanentFailed[email] = struct{}{}
	}
}

func (ac *AutoCrawler) GetShutdownRequested() *int32 {
	return &ac.shutdownRequested
}

func (ac *AutoCrawler) GetCrawler() *models.LinkedInCrawler {
	ac.crawlerMutex.RLock()
	defer ac.crawlerMutex.RUnlock()
	return ac.crawler
}

func (ac *AutoCrawler) SetCrawler(crawler *models.LinkedInCrawler) {
	ac.crawlerMutex.Lock()
	defer ac.crawlerMutex.Unlock()
	ac.crawler = crawler
}

func (ac *AutoCrawler) GetFileOpMutex() *sync.Mutex {
	return &ac.fileOpMutex
}
