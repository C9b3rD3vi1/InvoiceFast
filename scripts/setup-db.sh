#!/bin/bash
# InvoiceFast Database Setup Script
# Usage: ./scripts/setup-db.sh [dev|prod]

MODE=${1:-dev}

echo "=========================================="
echo "InvoiceFast Database Setup"
echo "Mode: $MODE"
echo "=========================================="

if [ "$MODE" = "dev" ]; then
    echo "Setting up SQLite for development..."
    export DB_DRIVER=sqlite3
    export DB_DSN=./data/invoicefast.db
    
    # Create data directory if it doesn't exist
    mkdir -p ./data
    
    echo "SQLite database will be created at: ./data/invoicefast.db"
    echo "To start dev server: go run ./cmd/server/main.go"

elif [ "$MODE" = "prod" ]; then
    echo "Setting up PostgreSQL for production..."
    export DB_DRIVER=postgres
    
    if [ -z "$DB_DSN" ]; then
        echo "ERROR: DB_DSN environment variable not set!"
        echo "Please set your PostgreSQL connection string:"
        echo "  export DB_DSN='postgresql://user:pass@host:5432/dbname?sslmode=disable'"
        exit 1
    fi
    
    echo "PostgreSQL DSN: ${DB_DSN%%@*}@..." # Hide password in output
    
    # Test connection
    echo "Testing PostgreSQL connection..."
    if command -v psql &> /dev/null; then
        psql "$DB_DSN" -c "SELECT version();" > /dev/null 2>&1
        if [ $? -ne 0 ]; then
            echo "ERROR: Could not connect to PostgreSQL"
            exit 1
        fi
        echo "PostgreSQL connection successful!"
    else
        echo "Warning: psql not found, skipping connection test"
    fi
    
    echo "To start production server: go run ./cmd/server/main.go"

else
    echo "ERROR: Invalid mode. Use 'dev' or 'prod'"
    echo "Usage: ./scripts/setup-db.sh [dev|prod]"
    exit 1
fi

echo ""
echo "=========================================="
echo "Environment configured successfully!"
echo "=========================================="
echo ""
echo "To run the application:"
echo "  go run ./cmd/server/main.go"
echo ""
echo "Environment variables being used:"
echo "  DB_DRIVER=$DB_DRIVER"
echo "  DB_DSN=${DB_DSN:0:30}..." 
echo ""