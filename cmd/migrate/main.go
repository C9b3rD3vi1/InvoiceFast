package main

import (
	"context"

	"invoicefast/internal/config"
	"invoicefast/internal/database"
	"invoicefast/internal/logger"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if exists (won't fail if missing)
	_ = godotenv.Load()

	cfg := config.Load()

	logSvc := logger.LoadFromConfig(cfg)
	logSvc.Info(context.Background(), "InvoiceFast: Starting migration",
		"mode", cfg.Server.Mode,
		"port", cfg.Server.Port,
	)

	db, err := database.New(&cfg.Database)
	if err != nil {
		logSvc.Fatal(context.Background(), "InvoiceFast: Database error", "error", err.Error())
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		logSvc.Fatal(context.Background(), "InvoiceFast: Migration failed", "error", err.Error())
	}

	logSvc.Info(context.Background(), "InvoiceFast: Migration completed successfully")
}
