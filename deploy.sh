#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}  InvoiceFast Deployment Script${NC}"
echo -e "${GREEN}=========================================${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
   echo -e "${RED}Please run as root${NC}"
   exit 1
fi

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Load environment variables
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

echo -e "${YELLOW}Step 1: Building Docker image...${NC}"
docker build -t invoicefast:latest .

echo -e "${YELLOW}Step 2: Checking Docker Compose...${NC}"
docker-compose up -d

echo -e "${YELLOW}Step 3: Running database migrations...${NC}"
docker-compose exec -T invoicefast ./migrate

echo -e "${YELLOW}Step 4: Checking health...${NC}"
sleep 10
curl -f http://localhost:8082/health || echo -e "${RED}Health check failed${NC}"

echo -e "${YELLOW}Step 5: Reloading Nginx...${NC}"
docker-compose exec -T nginx nginx -s reload

echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}  Deployment Complete!${NC}"
echo -e "${GREEN}=========================================${NC}"
echo ""
echo "Services:"
echo "  - InvoiceFast: http://localhost:8082"
echo "  - Nginx: http://localhost"
echo ""
echo "View logs: docker-compose logs -f"
echo "Stop services: docker-compose down"
