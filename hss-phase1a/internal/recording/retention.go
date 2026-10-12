package recording

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	"hss/internal/config"
	"hss/internal/database"
)

type RetentionEngine struct {
	MaxDiskPercent int
	RetentionDays  int
}

func NewRetentionEngine() *RetentionEngine {
	return &RetentionEngine{
		MaxDiskPercent: config.AppConfig.Storage.MaxDiskPercent,
		RetentionDays:  config.AppConfig.Storage.RetentionDays,
	}
}

// Start initiates the background disk monitoring loop.
func (re *RetentionEngine) Start(ctx context.Context) {
	log.Println("Starting Disk Retention Engine...")

	// Poll every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down Disk Retention Engine.")
			return
		case <-ticker.C:
			re.RunGarbageCollector(ctx)
			re.checkDiskUsage(ctx)
		}
	}
}

// RunGarbageCollector scans the SQLite database for recordings older than RetentionDays and deletes them.
func (re *RetentionEngine) RunGarbageCollector(ctx context.Context) {
	if database.DB == nil {
		log.Println("Retention GC bypassed: database is stubbed")
		return
	}
	if re.RetentionDays <= 0 {
		return // Retention disabled
	}

	cutoffTime := time.Now().Add(-1 * time.Duration(re.RetentionDays) * 24 * time.Hour)
	query := "SELECT id, path FROM recordings WHERE protected = 0 AND started_at < ?"

	rows, err := database.DB.Query(query, cutoffTime.Format(time.RFC3339))
	if err != nil {
		log.Printf("Error querying expired recordings: %v", err)
		return
	}
	defer rows.Close()

	var expiredIDs []string
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			continue
		}
		
		// Remove from filesystem
		if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("Error removing expired recording file %s: %v", path, err)
				continue
			}
		}
		
		expiredIDs = append(expiredIDs, id)
		log.Printf("Garbage Collector: Deleted expired recording %s (%s)", id, path)
	}

	// Delete from DB in batch
	for _, id := range expiredIDs {
		_, err = database.DB.Exec("DELETE FROM recordings WHERE id = ?", id)
		if err != nil {
			log.Printf("Error deleting expired recording record %s: %v", id, err)
		}
	}
}

func (re *RetentionEngine) checkDiskUsage(ctx context.Context) {
	recPath := config.AppConfig.Storage.RecordingsPath
	if recPath == "" {
		recPath = "./data/recordings"
	}

	var totalBytes int64
	_ = filepath.Walk(recPath, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			totalBytes += info.Size()
		}
		return nil
	})

	// Default 10 GB edge recording quota for percentage watermark calculation
	const quotaBytes = int64(10 * 1024 * 1024 * 1024)
	usagePercent := int((totalBytes * 100) / quotaBytes)

	if re.MaxDiskPercent > 0 && usagePercent > re.MaxDiskPercent {
		log.Printf("CRITICAL: Disk usage at %d%% (Max %d%%). Triggering retention cleanup!\n", usagePercent, re.MaxDiskPercent)
		re.cleanupOldest(ctx)
	} else if config.AppConfig.Storage.WarningDiskPercent > 0 && usagePercent > config.AppConfig.Storage.WarningDiskPercent {
		log.Printf("WARNING: Disk usage at %d%%. Approaching threshold.\n", usagePercent)
	}
}

func (re *RetentionEngine) cleanupOldest(ctx context.Context) {
	// Query SQLite for oldest unprotected recording
	query := "SELECT id, path FROM recordings WHERE protected = 0 ORDER BY started_at ASC LIMIT 1"
	var id, path string
	err := database.DB.QueryRow(query).Scan(&id, &path)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("No unprotected recordings to clean up.")
		} else {
			log.Printf("Error querying oldest recording: %v", err)
		}
		return
	}

	// Remove from filesystem
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error removing recording file %s: %v", path, err)
			return
		}
	}

	// Delete from DB
	_, err = database.DB.Exec("DELETE FROM recordings WHERE id = ?", id)
	if err != nil {
		log.Printf("Error deleting recording record %s: %v", id, err)
		return
	}

	log.Printf("Retention cleanup (disk full): Deleted recording %s (%s)", id, path)
}
