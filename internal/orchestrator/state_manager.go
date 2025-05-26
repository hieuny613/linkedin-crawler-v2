package orchestrator

import (
	"fmt"
)

// StateManager handles state persistence and management
type StateManager struct {
	autoCrawler *AutoCrawler
}

// NewStateManager creates a new StateManager instance
func NewStateManager(ac *AutoCrawler) *StateManager {
	return &StateManager{
		autoCrawler: ac,
	}
}

func (sm *StateManager) HasEmailsToProcess() bool {
	dbStorage := sm.autoCrawler.GetDBStorage()
	count, err := dbStorage.EmailRepo.CountRemainingEmails()
	if err != nil {
		return false
	}
	return count > 0
}

func (sm *StateManager) CountRemainingEmails() int {
	dbStorage := sm.autoCrawler.GetDBStorage()
	count, err := dbStorage.EmailRepo.CountRemainingEmails()
	if err != nil {
		return 0
	}
	return count
}

func (sm *StateManager) GetRemainingEmails() []string {
	dbStorage := sm.autoCrawler.GetDBStorage()
	emails, err := dbStorage.EmailRepo.GetRemainingEmails()
	if err != nil {
		return []string{}
	}
	return emails
}

// SaveStateOnShutdown saves the current state when shutting down
func (sm *StateManager) SaveStateOnShutdown() {
	withData, withoutData, failed, permanent := sm.autoCrawler.GetEmailMaps()
	totalEmails := sm.autoCrawler.GetTotalEmails()
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()
	config := sm.autoCrawler.GetConfig()
	fileOpMutex := sm.autoCrawler.GetFileOpMutex()

	// Calculate remaining emails
	var remainingEmails []string
	for _, email := range totalEmails {
		// If email is not successfully processed (both with data and without data) and not permanently failed
		if _, hasWithData := withData[email]; !hasWithData {
			if _, hasWithoutData := withoutData[email]; !hasWithoutData {
				if _, isPermanent := permanent[email]; !isPermanent {
					remainingEmails = append(remainingEmails, email)
				}
			}
		}
	}

	// Add failed emails (need retry)
	for email := range failed {
		found := false
		for _, existing := range remainingEmails {
			if existing == email {
				found = true
				break
			}
		}
		if !found {
			remainingEmails = append(remainingEmails, email)
		}
	}

	if len(remainingEmails) == 0 {
		fmt.Println("📝 Tất cả emails đã được xử lý")
		// Create empty file with thread-safe operation
		fileOpMutex.Lock()
		err := emailStorage.WriteEmailsToFile(config.EmailsFilePath, []string{})
		fileOpMutex.Unlock()
		if err != nil {
			fmt.Printf("⚠️ Không thể tạo file trống: %v\n", err)
		}
		return
	}

	// Write remaining emails to file using thread-safe operation
	fileOpMutex.Lock()
	err := emailStorage.WriteEmailsToFile(config.EmailsFilePath, remainingEmails)
	fileOpMutex.Unlock()
	if err != nil {
		fmt.Printf("⚠️ Không thể ghi emails file khi shutdown: %v\n", err)
		return
	}

	fmt.Printf("💾 Đã lưu %d emails chưa xử lý (Với data: %d, Không data: %d, Failed: %d, Permanent Failed: %d)\n",
		len(remainingEmails), len(withData), len(withoutData),
		len(failed), len(permanent))
}

// UpdateEmailsFile updates the emails file with current state
func (sm *StateManager) UpdateEmailsFile() {
	withData, withoutData, failed, permanent := sm.autoCrawler.GetEmailMaps()
	totalEmails := sm.autoCrawler.GetTotalEmails()
	emailStorage, _, _ := sm.autoCrawler.GetStorageServices()
	config := sm.autoCrawler.GetConfig()
	fileOpMutex := sm.autoCrawler.GetFileOpMutex()

	var remainingEmails []string

	// Add emails that haven't been processed successfully (both with data and without data) and not permanently failed
	for _, email := range totalEmails {
		if _, hasWithData := withData[email]; !hasWithData {
			if _, hasWithoutData := withoutData[email]; !hasWithoutData {
				if _, isPermanent := permanent[email]; !isPermanent {
					remainingEmails = append(remainingEmails, email)
				}
			}
		}
	}

	// Add failed emails (need retry)
	for email := range failed {
		found := false
		for _, existing := range remainingEmails {
			if existing == email {
				found = true
				break
			}
		}
		if !found {
			remainingEmails = append(remainingEmails, email)
		}
	}

	// Use thread-safe file operation
	fileOpMutex.Lock()
	err := emailStorage.WriteEmailsToFile(config.EmailsFilePath, remainingEmails)
	fileOpMutex.Unlock()
	if err != nil {
		fmt.Printf("⚠️ Không thể cập nhật emails file: %v\n", err)
	} else {
		fmt.Printf("💾 Đã cập nhật file emails: %d emails còn lại\n", len(remainingEmails))
	}
}
