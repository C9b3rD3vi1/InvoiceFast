// +build ignore

package main

// Migration: KRA eTIMS Compliance Upgrades
// Run this migration to add KRA compliance fields

import (
	"log"
	"path/filepath"
	"runtime"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/models"
)

func main() {
	_, currentFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(currentFile))

	cfg := config.Load(projectRoot + "/.env")
	db := database.Connect(cfg)
	
	log.Println("Running KRA compliance migration...")
	
	// Auto migrate new models
	err := db.AutoMigrate(
		&models.KRAAuditLog{},
	).Error
	
	if err != nil {
		log.Printf("Migration error: %v", err)
		return
	}
	
	log.Println("KRA compliance migration completed successfully!")
}