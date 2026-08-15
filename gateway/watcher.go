package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StartInboxWatcher starts a background goroutine that polls the inbox directory
// and ingests arriving files into an explicit tenant.
//
// The tenant is a required parameter, not a default. A background daemon has no
// request and therefore no principal, so the choice of where watched files
// belong is an operator decision made in configuration. Passing an empty tenant
// is a programming error and the watcher refuses to start.
func StartInboxWatcher(db *sql.DB, tenantID string, inboxDir string) {
	if tenantID == "" {
		log.Println("[SFTP Watcher] refusing to start: no tenant supplied")
		return
	}
	processedDir := filepath.Join(inboxDir, "processed")
	quarantineDir := filepath.Join(inboxDir, "quarantine")

	// Ensure directories exist
	_ = os.MkdirAll(inboxDir, 0755)
	_ = os.MkdirAll(processedDir, 0755)
	_ = os.MkdirAll(quarantineDir, 0755)

	fmt.Printf("[SFTP Watcher] Listening on directory: %s\n", inboxDir)

	go func() {
		for {
			time.Sleep(1 * time.Second)

			entries, err := os.ReadDir(inboxDir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				name := entry.Name()
				// Only process .ach, .txt, .csv, or .dat files
				ext := strings.ToLower(filepath.Ext(name))
				if ext != ".ach" && ext != ".txt" && ext != ".csv" && ext != ".dat" {
					continue
				}

				filePath := filepath.Join(inboxDir, name)
				content, err := os.ReadFile(filePath)
				if err != nil {
					log.Printf("[SFTP Watcher] Failed to read file %s: %v\n", name, err)
					continue
				}

				log.Printf("[SFTP Watcher] Detected new SFTP file drop: %s (%d bytes). Processing...\n", name, len(content))

				result, err := ProcessFileBytes(db, tenantID, name, content)
				if err != nil {
					log.Printf("[SFTP Watcher] Ingestion error for %s: %v\n", name, err)
					continue
				}

				log.Printf("[SFTP Watcher] File %s processed -> Status: %s, SHA256: %s, Findings: %d\n",
					name, result.Status, result.Hash, len(result.Findings))

				// Move file based on validation outcome
				var destPath string
				if result.Status == "RELEASED" {
					destPath = filepath.Join(processedDir, fmt.Sprintf("%d_%s", time.Now().Unix(), name))
				} else {
					destPath = filepath.Join(quarantineDir, fmt.Sprintf("%d_%s", time.Now().Unix(), name))
				}

				_ = os.Rename(filePath, destPath)
			}
		}
	}()
}
