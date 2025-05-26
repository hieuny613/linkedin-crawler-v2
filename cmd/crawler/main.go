package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"linkedin-crawler/internal/config"
	"linkedin-crawler/internal/orchestrator"
	"linkedin-crawler/internal/utils"
)

func main() {
	fmt.Println("🚀 LinkedIn Auto Crawler - SQLite Version")
	fmt.Println(strings.Repeat("=", 60))

	// Load configuration
	cfg := config.DefaultConfig()

	// Create auto crawler with SQLite support
	autoCrawler, err := orchestrator.New(cfg)
	if err != nil {
		log.Fatalf("❌ Lỗi khởi tạo auto crawler: %v", err)
	}

	// Start crawling
	startTime := time.Now()
	err = autoCrawler.Run()
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("❌ Lỗi trong quá trình chạy: %v", err)
	}

	fmt.Printf("🎉 Hoàn thành trong %s\n", utils.FormatDuration(duration))
	fmt.Printf("📊 Kết quả được lưu trong file: %s\n", autoCrawler.GetOutputFile())
	fmt.Printf("💾 Database: crawler.db\n")

	// Memory stats để kiểm tra memory improvements
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("💾 Memory: Alloc=%d KB, TotalAlloc=%d KB, Sys=%d KB, NumGC=%d\n",
		m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC)

	fmt.Println(strings.Repeat("=", 60))
}
