package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"invoicefast/internal/logger"
)

type migration struct {
	Version  string
	Name     string
	SQL      string
}

type schemaMigration struct {
	Version   string `gorm:"primaryKey"`
	AppliedAt time.Time
}

func (schemaMigration) TableName() string {
	return "schema_migrations"
}

func (db *DB) RunMigrations(migrationsDir string) error {
	log := logger.Get()

	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn(context.Background(), "Migrations directory not found", "dir", migrationsDir)
			return nil
		}
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []migration
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(f.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version := parts[0]
		name := strings.TrimSuffix(parts[1], ".sql")

		content, err := os.ReadFile(filepath.Join(migrationsDir, f.Name()))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", f.Name(), err)
		}

		migrations = append(migrations, migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	var applied []schemaMigration
	if err := db.Order("version ASC").Find(&applied).Error; err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}

	appliedMap := make(map[string]bool)
	for _, m := range applied {
		appliedMap[m.Version] = true
	}

	for _, m := range migrations {
		if appliedMap[m.Version] {
			log.Debug(context.Background(), "Migration already applied", "version", m.Version, "name", m.Name)
			continue
		}

		log.Info(context.Background(), "Applying migration", "version", m.Version, "name", m.Name)

		statements := splitSQLStatements(m.SQL)
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if db.isPostgres {
				stmt = strings.ReplaceAll(stmt, "DATETIME", "TIMESTAMP WITH TIME ZONE")
			} else {
				stmt = strings.ReplaceAll(stmt, "TIMESTAMP WITH TIME ZONE", "DATETIME")
				stmt = strings.ReplaceAll(stmt, "JSONB", "TEXT")
				stmt = strings.ReplaceAll(stmt, "uuid_generate_v4()", "(lower(hex(randomblob(16))))")
				if strings.Contains(stmt, "CREATE EXTENSION") || strings.Contains(stmt, "CREATE OR REPLACE FUNCTION") || strings.Contains(stmt, "CREATE TRIGGER") {
					log.Debug(context.Background(), "Skipping PostgreSQL-specific statement for SQLite", "version", m.Version)
					continue
				}
			}
			if err := db.Exec(stmt).Error; err != nil {
				return fmt.Errorf("migration %s failed: %w\nSQL: %s", m.Version, err, stmt)
			}
		}

		record := schemaMigration{
			Version:   m.Version,
			AppliedAt: time.Now().UTC(),
		}
		if err := db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to record migration %s: %w", m.Version, err)
		}

		log.Info(context.Background(), "Migration applied", "version", m.Version, "name", m.Name)
	}

	return nil
}

func splitSQLStatements(sql string) []string {
	var statements []string
	current := strings.Builder{}
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		if inString {
			current.WriteByte(ch)
			if ch == stringChar && (i+1 >= len(sql) || sql[i+1] != stringChar) {
				inString = false
			}
			continue
		}

		if ch == '\'' || ch == '"' {
			inString = true
			stringChar = ch
			current.WriteByte(ch)
			continue
		}

		if ch == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}
