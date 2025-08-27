#!/bin/bash

# Nutreek Production Monitoring Script
# This script monitors the health and performance of the deployed application

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
APP_URL="${APP_URL:-http://localhost}"
API_URL="${API_URL:-http://localhost/api}"
NOTIFICATION_EMAIL="${NOTIFICATION_EMAIL:-admin@yourdomain.com}"
LOG_FILE="${LOG_FILE:-logs/monitor.log}"

# Create log directory if it doesn't exist
mkdir -p "$(dirname "$LOG_FILE")"

# Function to log messages
log() {
    local message="$1"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$timestamp] $message" >> "$LOG_FILE"
    echo -e "${BLUE}[$timestamp]${NC} $message"
}

# Function to send alerts
send_alert() {
    local subject="$1"
    local message="$2"

    log "ALERT: $subject - $message"

    # Send email alert if configured
    if command -v mail &> /dev/null && [ -n "$NOTIFICATION_EMAIL" ]; then
        echo "$message" | mail -s "$subject" "$NOTIFICATION_EMAIL"
    fi
}

# Function to check HTTP endpoint
check_http() {
    local url="$1"
    local service_name="$2"
    local timeout="${3:-10}"

    if curl -f -s --max-time "$timeout" "$url" > /dev/null; then
        echo -e "${GREEN}✅${NC} $service_name is healthy"
        return 0
    else
        echo -e "${RED}❌${NC} $service_name is unhealthy"
        return 1
    fi
}

# Function to check database connection
check_database() {
    local db_host="${DB_HOST:-localhost}"
    local db_port="${DB_PORT:-5432}"
    local db_name="${DB_NAME:-nutrition_prod}"

    if command -v pg_isready &> /dev/null; then
        if pg_isready -h "$db_host" -p "$db_port" -d "$db_name" &> /dev/null; then
            echo -e "${GREEN}✅${NC} Database is healthy"
            return 0
        fi
    fi

    echo -e "${RED}❌${NC} Database is unhealthy"
    return 1
}

# Function to check Redis connection
check_redis() {
    local redis_host="${REDIS_HOST:-localhost}"
    local redis_port="${REDIS_PORT:-6379}"

    if command -v redis-cli &> /dev/null; then
        if timeout 5 redis-cli -h "$redis_host" -p "$redis_port" ping &> /dev/null; then
            echo -e "${GREEN}✅${NC} Redis is healthy"
            return 0
        fi
    fi

    echo -e "${RED}❌${NC} Redis is unhealthy"
    return 1
}

# Function to check Docker containers
check_docker_containers() {
    local unhealthy_containers=()

    # Check if docker is available
    if ! command -v docker &> /dev/null; then
        echo -e "${YELLOW}⚠️${NC} Docker command not available"
        return 0
    fi

    # Get container names and statuses
    while IFS= read -r line; do
        container_name=$(echo "$line" | awk '{print $NF}')
        container_status=$(echo "$line" | awk '{print $(NF-1)}')

        if [[ "$container_status" != "Up" ]]; then
            unhealthy_containers+=("$container_name")
            echo -e "${RED}❌${NC} Container $container_name is not healthy"
        else
            echo -e "${GREEN}✅${NC} Container $container_name is healthy"
        fi
    done < <(docker ps --format "table {{.Status}}\t{{.Names}}" | tail -n +2)

    if [ ${#unhealthy_containers[@]} -gt 0 ]; then
        return 1
    fi

    return 0
}

# Function to check disk space
check_disk_space() {
    local threshold="${DISK_THRESHOLD:-90}"
    local disk_usage=$(df / | tail -1 | awk '{print $5}' | sed 's/%//')

    if [ "$disk_usage" -gt "$threshold" ]; then
        echo -e "${RED}❌${NC} Disk usage is ${disk_usage}% (threshold: ${threshold}%)"
        return 1
    else
        echo -e "${GREEN}✅${NC} Disk usage is ${disk_usage}% (threshold: ${threshold}%)"
        return 0
    fi
}

# Function to check memory usage
check_memory() {
    local threshold="${MEMORY_THRESHOLD:-90}"
    local memory_usage=$(free | grep Mem | awk '{printf "%.0f", $3/$2 * 100.0}')

    if [ "$memory_usage" -gt "$threshold" ]; then
        echo -e "${RED}❌${NC} Memory usage is ${memory_usage}% (threshold: ${threshold}%)"
        return 1
    else
        echo -e "${GREEN}✅${NC} Memory usage is ${memory_usage}% (threshold: ${threshold}%)"
        return 0
    fi
}

# Function to perform comprehensive health check
perform_health_check() {
    log "Starting comprehensive health check"

    local issues_found=0

    echo "=== Application Health ==="
    if ! check_http "$APP_URL" "Frontend"; then
        ((issues_found++))
        send_alert "Frontend Unhealthy" "The frontend application is not responding at $APP_URL"
    fi

    if ! check_http "$API_URL/health" "Backend API"; then
        ((issues_found++))
        send_alert "Backend API Unhealthy" "The backend API is not responding at $API_URL/health"
    fi

    echo ""
    echo "=== Infrastructure Health ==="
    if ! check_database; then
        ((issues_found++))
        send_alert "Database Unhealthy" "The database connection is failing"
    fi

    if ! check_redis; then
        ((issues_found++))
        send_alert "Redis Unhealthy" "The Redis cache is not responding"
    fi

    echo ""
    echo "=== System Resources ==="
    if ! check_disk_space; then
        ((issues_found++))
        send_alert "High Disk Usage" "Disk usage is above the threshold"
    fi

    if ! check_memory; then
        ((issues_found++))
        send_alert "High Memory Usage" "Memory usage is above the threshold"
    fi

    echo ""
    echo "=== Docker Containers ==="
    if ! check_docker_containers; then
        ((issues_found++))
        send_alert "Docker Issues" "Some Docker containers are not healthy"
    fi

    if [ $issues_found -eq 0 ]; then
        log "All systems healthy"
        echo -e "${GREEN}🎉 All systems are healthy!${NC}"
    else
        log "Found $issues_found health issues"
        echo -e "${RED}⚠️ Found $issues_found health issues that need attention${NC}"
    fi

    return $issues_found
}

# Function to generate system report
generate_report() {
    local report_file="reports/health_report_$(date +%Y%m%d_%H%M%S).txt"
    mkdir -p reports

    {
        echo "Nutreek Health Report"
        echo "Generated: $(date)"
        echo "======================"
        echo ""

        echo "System Information:"
        echo "  OS: $(uname -s) $(uname -r)"
        echo "  Uptime: $(uptime -p)"
        echo "  Load Average: $(uptime | awk -F'load average:' '{print $2}')"
        echo ""

        echo "Application Status:"
        echo "  Frontend URL: $APP_URL"
        echo "  API URL: $API_URL"
        echo ""

        echo "Resource Usage:"
        echo "  CPU: $(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print 100 - $1"%"}')"
        echo "  Memory: $(free -h | grep '^Mem:' | awk '{print $3 "/" $2}')"
        echo "  Disk: $(df -h / | awk 'NR==2 {print $3 "/" $2 " (" $5 ")"}')"
        echo ""

    } > "$report_file"

    log "Health report generated: $report_file"
    echo "Health report saved to: $report_file"
}

# Function to cleanup old log files
cleanup_logs() {
    local retention_days="${LOG_RETENTION_DAYS:-30}"

    # Cleanup monitor logs
    find "$(dirname "$LOG_FILE")" -name "monitor.log.*" -mtime +"$retention_days" -delete

    # Cleanup health reports
    find reports -name "health_report_*.txt" -mtime +"$retention_days" -delete

    log "Cleaned up old log files (retention: ${retention_days} days)"
}

# Main monitoring function
main() {
    echo "Nutreek Production Monitoring"
    echo "============================"

    local command="${1:-check}"

    case $command in
        "check")
            perform_health_check
            ;;
        "report")
            perform_health_check
            generate_report
            ;;
        "cleanup")
            cleanup_logs
            ;;
        "continuous")
            echo "Starting continuous monitoring (press Ctrl+C to stop)..."
            while true; do
                perform_health_check
                echo ""
                sleep "${MONITOR_INTERVAL:-300}"  # Default 5 minutes
            done
            ;;
        *)
            echo "Usage: $0 {check|report|cleanup|continuous}"
            echo ""
            echo "Commands:"
            echo "  check      - Perform one-time health check"
            echo "  report     - Generate health report"
            echo "  cleanup    - Clean up old log files"
            echo "  continuous - Continuous monitoring"
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"