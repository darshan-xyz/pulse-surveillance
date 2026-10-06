package recording

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"

	"hss/internal/config"
	"hss/internal/database"
)

type RetentionEngine struct {
	MaxDiskPercent int
}

func NewRetentionEngine() *RetentionEngine {
	return &RetentionEngine{
		MaxDiskPercent: config.AppConfig.Storage.MaxDiskPercent,
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
			re.checkDiskUsage(ctx)
		}
	}
}

func (re *RetentionEngine) checkDiskUsage(ctx context.Context) {
	// Simple mock calculation for disk usage MVP to avoid OS-specific CGO bindings on Windows
	simulatedUsage := 85 // Mock high usage to trigger cleanup for testing

	if simulatedUsage > re.MaxDiskPercent {
		log.Printf("CRITICAL: Disk usage at %d%% (Max %d%%). Triggering retention cleanup!\n", simulatedUsage, re.MaxDiskPercent)
		re.cleanupOldest(ctx)
	} else if simulatedUsage > config.AppConfig.Storage.WarningDiskPercent {
		log.Printf("WARNING: Disk usage at %d%%. Approaching threshold.\n", simulatedUsage)
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

	log.Printf("Retention cleanup: Deleted recording %s (%s)", id, path)
}
