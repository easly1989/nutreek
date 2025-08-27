#!/bin/bash

# E2E Test Setup Script
# This script sets up the environment for running e2e tests

set -e

echo "🚀 Setting up E2E test environment..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required tools are installed
check_dependencies() {
    print_status "Checking dependencies..."

    if ! command -v node &> /dev/null; then
        print_error "Node.js is not installed"
        exit 1
    fi

    if ! command -v npm &> /dev/null; then
        print_error "npm is not installed"
        exit 1
    fi

    if ! command -v npx &> /dev/null; then
        print_error "npx is not installed"
        exit 1
    fi

    print_status "Dependencies check passed"
}

# Generate Prisma client
generate_prisma() {
    print_status "Generating Prisma client..."
    cd backend
    npx prisma generate
    cd ..
    print_status "Prisma client generated successfully"
}

# Function to cleanup background processes
cleanup() {
    print_status "Cleaning up..."
    # Kill background processes
    kill $(jobs -p) 2>/dev/null || true
    # Wait a moment for cleanup
    sleep 2
    print_status "Cleanup completed"
}

# Trap to ensure cleanup on script exit
trap cleanup EXIT

# Check if ports are available
check_ports() {
    print_status "Checking if required ports are available..."

    # Check if port 3000 is in use
    if lsof -Pi :3000 -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_warning "Port 3000 is already in use (frontend)"
        read -p "Do you want to continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_status "E2E setup cancelled"
            exit 1
        fi
    fi

    # Check if port 4000 is in use
    if lsof -Pi :4000 -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_warning "Port 4000 is already in use (backend)"
        read -p "Do you want to continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_status "E2E setup cancelled"
            exit 1
        fi
    fi

    print_status "Port check completed"
}

# Start backend server
start_backend() {
    print_status "Starting backend server..."
    cd backend
    npm run start:dev &
    BACKEND_PID=$!
    cd ..

    # Wait for backend to be ready
    print_status "Waiting for backend server to be ready..."
    for i in {1..30}; do
        if curl -s http://localhost:4000/health >/dev/null 2>&1; then
            print_status "Backend server is ready"
            break
        fi
        sleep 2
        if [ $i -eq 30 ]; then
            print_error "Backend server failed to start within 60 seconds"
            exit 1
        fi
    done
}

# Start frontend server
start_frontend() {
    print_status "Starting frontend server..."
    cd frontend
    npm run dev &
    FRONTEND_PID=$!
    cd ..

    # Wait for frontend to be ready
    print_status "Waiting for frontend server to be ready..."
    for i in {1..30}; do
        if curl -s http://localhost:3000 >/dev/null 2>&1; then
            print_status "Frontend server is ready"
            break
        fi
        sleep 2
        if [ $i -eq 30 ]; then
            print_error "Frontend server failed to start within 60 seconds"
            exit 1
        fi
    done
}

# Run e2e tests
run_e2e_tests() {
    print_status "Running E2E tests..."
    npm run test:e2e
}

# Main execution
main() {
    print_status "Starting E2E test setup..."

    check_dependencies
    check_ports
    generate_prisma
    start_backend
    start_frontend

    print_status "Both servers are running:"
    print_status "  - Frontend: http://localhost:3000"
    print_status "  - Backend: http://localhost:4000"

    # Run the tests
    run_e2e_tests

    print_status "E2E test setup completed successfully!"
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi