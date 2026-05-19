// +build ignore

package main

// Migration: KRA eTIMS Compliance Upgrades
// Run this migration to add KRA compliance fields

import (
	"context"
	"path/filepath"
	"runtime"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/logger"
	"invoicefast/internal/models"
)

func main() {
	_, currentFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(currentFile))

	cfg := config.Load(projectRoot + "/.env")
	db := database.Connect(cfg)
	
	logger.Get().Info(context.Background(), "Running KRA compliance migration")
	
	// Auto migrate new models
	err := db.AutoMigrate(
		&models.KRAAuditLog{},
	).Error
	
	if err != nil {
		logger.Get().Error(context.Background(), "Migration error", "error", err)
		return
	}
	
	logger.Get().Info(context.Background(), "KRA compliance migration completed successfully!")
}