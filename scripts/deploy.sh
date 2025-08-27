#!/bin/bash

# Nutreek Production Deployment Script
# This script handles the complete deployment process for the Nutreek application

set -e  # Exit on any error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="nutreek"
DOCKER_REGISTRY="${DOCKER_REGISTRY:-your-registry.com}"
ENVIRONMENT="${ENVIRONMENT:-production}"

echo -e "${BLUE}🚀 Starting Nutreek deployment to ${ENVIRONMENT}${NC}"

# Function to print step information
print_step() {
    echo -e "${YELLOW}[STEP]${NC} $1"
}

# Function to print success messages
print_success() {
    echo -e "${GREEN}✅${NC} $1"
}

# Function to print error messages
print_error() {
    echo -e "${RED}❌ ERROR:${NC} $1"
}

# Check if required tools are installed
check_dependencies() {
    print_step "Checking dependencies..."

    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed or not in PATH"
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed or not in PATH"
        exit 1
    fi

    print_success "All dependencies are available"
}

# Build Docker images
build_images() {
    print_step "Building Docker images..."

    # Build backend image
    print_step "Building backend image..."
    docker build -f backend/Dockerfile.prod -t ${DOCKER_REGISTRY}/${PROJECT_NAME}-backend:latest ./backend

    # Build frontend image
    print_step "Building frontend image..."
    docker build -f frontend/Dockerfile.prod -t ${DOCKER_REGISTRY}/${PROJECT_NAME}-frontend:latest ./frontend

    print_success "Docker images built successfully"
}

# Push images to registry (optional)
push_images() {
    if [ "${PUSH_IMAGES:-false}" = "true" ]; then
        print_step "Pushing images to registry..."

        docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}-backend:latest
        docker push ${DOCKER_REGISTRY}/${PROJECT_NAME}-frontend:latest

        print_success "Images pushed to registry"
    fi
}

# Deploy to production
deploy_production() {
    print_step "Deploying to production environment..."

    # Create environment file if it doesn't exist
    if [ ! -f ".env.prod" ]; then
        print_step "Creating production environment file..."
        cp .env.example .env.prod
        print_error "Please edit .env.prod with your production configuration before proceeding"
        exit 1
    fi

    # Stop existing containers
    print_step "Stopping existing containers..."
    docker-compose -f docker-compose.prod.yml down || true

    # Start new containers
    print_step "Starting production containers..."
    docker-compose -f docker-compose.prod.yml up -d

    # Wait for services to be healthy
    print_step "Waiting for services to be healthy..."
    sleep 30

    # Run database migrations
    print_step "Running database migrations..."
    docker-compose -f docker-compose.prod.yml exec -T backend npx prisma migrate deploy

    # Generate Prisma client
    print_step "Generating Prisma client..."
    docker-compose -f docker-compose.prod.yml exec -T backend npx prisma generate

    print_success "Production deployment completed successfully"
}

# Run health checks
run_health_checks() {
    print_step "Running health checks..."

    # Wait for services to be ready
    local retries=30
    local wait_time=10

    for i in $(seq 1 $retries); do
        echo "Health check attempt $i/$retries..."

        # Check backend health
        if curl -f -s http://localhost/api/health > /dev/null; then
            print_success "Backend is healthy"
            break
        else
            if [ $i -eq $retries ]; then
                print_error "Backend health check failed after $retries attempts"
                exit 1
            fi
            sleep $wait_time
        fi
    done

    print_success "All health checks passed"
}

# Backup database
backup_database() {
    print_step "Creating database backup..."

    local backup_dir="./backups"
    mkdir -p "$backup_dir"

    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="$backup_dir/backup_$timestamp.sql"

    docker-compose -f docker-compose.prod.yml exec -T db pg_dump -U postgres nutrition_prod > "$backup_file"

    print_success "Database backup created: $backup_file"
}

# Main deployment process
main() {
    echo "Nutreek Production Deployment Script"
    echo "=================================="

    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --push-images)
                PUSH_IMAGES=true
                shift
                ;;
            --skip-build)
                SKIP_BUILD=true
                shift
                ;;
            --backup)
                CREATE_BACKUP=true
                shift
                ;;
            *)
                print_error "Unknown option: $1"
                echo "Usage: $0 [--push-images] [--skip-build] [--backup]"
                exit 1
                ;;
        esac
    done

    # Run deployment steps
    check_dependencies

    if [ "${SKIP_BUILD:-false}" != "true" ]; then
        build_images
        push_images
    fi

    if [ "${CREATE_BACKUP:-false}" = "true" ]; then
        backup_database
    fi

    deploy_production
    run_health_checks

    print_success "🎉 Deployment completed successfully!"
    echo ""
    echo "Application URLs:"
    echo "  Frontend: http://localhost"
    echo "  Backend API: http://localhost/api"
    echo "  API Documentation: http://localhost/api/docs"
    echo ""
    echo "To view logs:"
    echo "  docker-compose -f docker-compose.prod.yml logs -f"
    echo ""
    echo "To scale services:"
    echo "  docker-compose -f docker-compose.prod.yml up -d --scale frontend=3"
}

# Run main function with all arguments
main "$@"