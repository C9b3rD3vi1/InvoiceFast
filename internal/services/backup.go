package services

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"invoicefast/internal/config"
	"invoicefast/internal/logger"
)

type BackupService struct {
	cfg     *config.BackupConfig
	dbPath  string
	ticker  *time.Ticker
	stopCh  chan struct{}
}

func NewBackupService(cfg *config.BackupConfig, dbPath string) *BackupService {
	return &BackupService{
		cfg:    cfg,
		dbPath: dbPath,
		stopCh: make(chan struct{}),
	}
}

func (s *BackupService) Start() {
	if !s.cfg.Enabled {
		return
	}

	schedule, err := time.ParseDuration(s.cfg.Schedule)
	if err != nil {
		schedule = 24 * time.Hour
	}

	s.ticker = time.NewTicker(schedule)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Get().Error(context.Background(), "panic recovered", "category", "panic", "recover", r)
			}
		}()
		ctx := context.Background()
		logger.Get().Info(ctx, "Backup service started", "schedule", schedule.String())

		if err := s.RunBackup(ctx); err != nil {
			logger.Get().Error(ctx, "Backup service: initial backup failed", "error", err)
		}

		for {
			select {
			case <-s.ticker.C:
				if err := s.RunBackup(ctx); err != nil {
					logger.Get().Error(ctx, "Backup service: scheduled backup failed", "error", err)
				}
			case <-s.stopCh:
				s.ticker.Stop()
				return
			}
		}
	}()
}

func (s *BackupService) Stop() {
	if s.ticker != nil {
		close(s.stopCh)
	}
}

func (s *BackupService) RunBackup(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	timestamp := time.Now().Format("2006-01-02T150405")
	backupName := fmt.Sprintf("invoicefast-%s.db.gz", timestamp)
	backupPath := filepath.Join(s.cfg.LocalDir, backupName)

	if err := os.MkdirAll(s.cfg.LocalDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	logger.Get().Info(ctx, "Backup: starting database backup", "path", backupPath)

	if strings.Contains(s.dbPath, ".db") {
		if err := s.backupSQLite(ctx, backupPath); err != nil {
			return fmt.Errorf("sqlite backup failed: %w", err)
		}
	} else {
		if err := s.backupPostgres(ctx, backupPath); err != nil {
			return fmt.Errorf("postgres backup failed: %w", err)
		}
	}

	logger.Get().Info(ctx, "Backup: database backup completed", "path", backupPath)

	if s.cfg.S3Bucket != "" {
		if err := s.uploadToS3(ctx, backupPath); err != nil {
			logger.Get().Error(ctx, "Backup: S3 upload failed", "error", err)
		}
	}

	if err := s.pruneBackups(ctx); err != nil {
		logger.Get().Error(ctx, "Backup: pruning failed", "error", err)
	}

	return nil
}

func (s *BackupService) backupSQLite(ctx context.Context, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	srcFile, err := os.Open(s.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer destFile.Close()

	gzWriter, err := gzip.NewWriterLevel(destFile, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzWriter.Close()

	written, err := io.Copy(gzWriter, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy and compress database: %w", err)
	}

	logger.Get().Info(ctx, "Backup: SQLite backup compressed", "bytes", written, "path", destPath)
	return nil
}

func (s *BackupService) backupPostgres(ctx context.Context, destPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	cmd := exec.CommandContext(ctx, "pg_dump", s.dbPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer destFile.Close()

	gzWriter, err := gzip.NewWriterLevel(destFile, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzWriter.Close()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pg_dump: %w", err)
	}

	written, err := io.Copy(gzWriter, stdout)
	if err != nil {
		return fmt.Errorf("failed to compress pg_dump output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	logger.Get().Info(ctx, "Backup: PostgreSQL backup compressed", "bytes", written, "path", destPath)
	return nil
}

func (s *BackupService) uploadToS3(ctx context.Context, filePath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.cfg.S3Bucket == "" {
		logger.Get().Info(ctx, "Backup: S3 not configured, skipping upload")
		return nil
	}

	args := []string{"s3", "cp", filePath, fmt.Sprintf("s3://%s/%s", s.cfg.S3Bucket, filepath.Base(filePath))}
	if s.cfg.S3Endpoint != "" {
		args = append(args, "--endpoint-url", s.cfg.S3Endpoint)
	}

	logger.Get().Info(ctx, "Backup: uploading to S3", "bucket", s.cfg.S3Bucket, "file", filePath, "args", args)

	if _, err := exec.LookPath("aws"); err != nil {
		logger.Get().Warn(ctx, "Backup: aws CLI not found, skipping S3 upload", "error", err)
		return nil
	}

	cmd := exec.CommandContext(ctx, "aws", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aws s3 cp failed: %w, output: %s", err, string(out))
	}

	logger.Get().Info(ctx, "Backup: S3 upload completed", "bucket", s.cfg.S3Bucket, "file", filePath, "output", string(out))
	return nil
}

func (s *BackupService) pruneBackups(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	entries, err := os.ReadDir(s.cfg.LocalDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []string
	prefix := "invoicefast-"
	suffix := ".db.gz"

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			backups = append(backups, filepath.Join(s.cfg.LocalDir, name))
		}
	}

	if len(backups) <= s.cfg.RetentionDays {
		return nil
	}

	sort.Strings(backups)

	toDelete := backups[:len(backups)-s.cfg.RetentionDays]

	for _, path := range toDelete {
		if err := os.Remove(path); err != nil {
			logger.Get().Error(ctx, "Backup: failed to remove old backup", "path", path, "error", err)
			continue
		}
		logger.Get().Info(ctx, "Backup: pruned old backup", "path", path)
	}

	return nil
}
