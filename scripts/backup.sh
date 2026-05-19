#!/bin/bash

# InvoiceFast Database Backup Script
# Usage: ./backup.sh [keep_days]
# Example: ./backup.sh 30 (keeps backups for 30 days)

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
KEEP_DAYS="${1:-30}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Database configuration (can be overridden by environment)
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-invoicefast}"
DB_USER="${DB_USER:-invoicefast}"
DB_PASSWORD="${DB_PASSWORD:-}"

# S3 configuration (optional)
S3_BUCKET="${S3_BUCKET:-}"
S3_PREFIX="${S3_PREFIX:-invoicefast-backups}"

# Logging
LOG_FILE="${BACKUP_DIR}/backup.log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" | tee -a "$LOG_FILE" >&2
}

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Determine backup type and run appropriate backup
run_postgres_backup() {
    local backup_file="${BACKUP_DIR}/invoicefast_${TIMESTAMP}.sql.gz"
    
    log "Starting PostgreSQL backup..."
    
    # Set password if provided
    if [ -n "$DB_PASSWORD" ]; then
        export PGPASSWORD="$DB_PASSWORD"
    fi
    
    # Run pg_dump
    if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" | gzip > "$backup_file"; then
        log "Backup created: $backup_file"
        
        # Get file size
        local size=$(du -h "$backup_file" | cut -f1)
        log "Backup size: $size"
        
        # Upload to S3 if configured
        if [ -n "$S3_BUCKET" ]; then
            upload_to_s3 "$backup_file"
        fi
        
        # Verify backup
        if verify_backup "$backup_file"; then
            log "Backup verified successfully"
        else
            log_error "Backup verification failed"
            return 1
        fi
    else
        log_error "Backup failed"
        return 1
    fi
    
    echo "$backup_file"
}

run_sqlite_backup() {
    local backup_file="${BACKUP_DIR}/invoicefast_${TIMESTAMP}.db.gz"
    local db_path="${DB_PATH:-./data/invoicefast.db}"
    
    log "Starting SQLite backup..."
    
    if [ ! -f "$db_path" ]; then
        log_error "Database file not found: $db_path"
        return 1
    fi
    
    # Copy and compress
    if gzip -c "$db_path" > "$backup_file"; then
        log "Backup created: $backup_file"
        
        local size=$(du -h "$backup_file" | cut -f1)
        log "Backup size: $size"
        
        # Upload to S3 if configured
        if [ -n "$S3_BUCKET" ]; then
            upload_to_s3 "$backup_file"
        fi
        
        # Verify backup
        if verify_backup "$backup_file"; then
            log "Backup verified successfully"
        else
            log_error "Backup verification failed"
            return 1
        fi
    else
        log_error "Backup failed"
        return 1
    fi
    
    echo "$backup_file"
}

upload_to_s3() {
    local file="$1"
    local s3_key="${S3_PREFIX}/$(basename "$file")"
    
    log "Uploading to S3: s3://${S3_BUCKET}/${s3_key}"
    
    if command -v aws &> /dev/null; then
        aws s3 cp "$file" "s3://${S3_BUCKET}/${s3_key}" 
        log "Upload complete"
    else
        log_error "AWS CLI not found, skipping S3 upload"
        return 1
    fi
}

verify_backup() {
    local file="$1"
    
    # Check file exists and has content
    if [ ! -s "$file" ]; then
        log_error "Backup file is empty"
        return 1
    fi
    
    # Check if it's a valid gzip file
    if ! gzip -t "$file" 2>/dev/null; then
        log_error "Backup file is corrupted (not valid gzip)"
        return 1
    fi
    
    return 0
}

cleanup_old_backups() {
    log "Cleaning up backups older than ${KEEP_DAYS} days..."
    
    find "$BACKUP_DIR" -name "*.sql.gz" -mtime +"$KEEP_DAYS" -delete
    find "$BACKUP_DIR" -name "*.db.gz" -mtime +"$KEEP_DAYS" -delete
    
    # Also cleanup S3 if configured
    if [ -n "$S3_BUCKET" ] && command -v aws &> /dev/null; then
        local cutoff_date=$(date -d "$KEEP_DAYS days ago" +%Y-%m-%d)
        aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" | while read -r line; do
            local backup_date=$(echo "$line" | awk '{print $1}')
            if [[ "$backup_date" < "$cutoff_date" ]]; then
                local backup_name=$(echo "$line" | awk '{print $4}')
                aws s3 rm "s3://${S3_BUCKET}/${S3_PREFIX}/${backup_name}"
                log "Deleted old S3 backup: $backup_name"
            fi
        done
    fi
    
    log "Cleanup complete"
}

# Main execution
main() {
    local db_driver="${DB_DRIVER:-sqlite3}"
    local backup_file=""
    
    log "=========================================="
    log "InvoiceFast Backup Starting"
    log "=========================================="
    log "Database driver: $db_driver"
    log "Backup directory: $BACKUP_DIR"
    log "Keep days: $KEEP_DAYS"
    
    case "$db_driver" in
        postgres|postgresql)
            backup_file=$(run_postgres_backup)
            ;;
        sqlite|sqlite3)
            backup_file=$(run_sqlite_backup)
            ;;
        *)
            log_error "Unsupported database driver: $db_driver"
            exit 1
            ;;
    esac
    
    if [ -n "$backup_file" ]; then
        cleanup_old_backups
        
        log "=========================================="
        log "Backup Completed Successfully"
        log "=========================================="
    else
        log_error "Backup failed"
        exit 1
    fi
}

main "$@"