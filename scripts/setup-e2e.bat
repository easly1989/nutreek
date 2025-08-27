@echo off
REM E2E Test Setup Script for Windows
REM This script sets up the environment for running e2e tests

echo 🚀 Setting up E2E test environment...

REM Check if required tools are installed
echo [INFO] Checking dependencies...
where node >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Node.js is not installed
    exit /b 1
)

where npm >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] npm is not installed
    exit /b 1
)

echo [INFO] Dependencies check passed

REM Generate Prisma client
echo [INFO] Generating Prisma client...
cd backend
npx prisma generate
if %errorlevel% neq 0 (
    echo [ERROR] Failed to generate Prisma client
    cd ..
    exit /b 1
)
cd ..
echo [INFO] Prisma client generated successfully

REM Check if ports are available
echo [INFO] Checking if required ports are available...
netstat -ano | findstr :3000 >nul 2>nul
if %errorlevel% equ 0 (
    echo [WARN] Port 3000 is already in use (frontend)
    set /p choice="Do you want to continue anyway? (y/N): "
    if /i not "!choice!"=="y" (
        echo [INFO] E2E setup cancelled
        exit /b 1
    )
)

netstat -ano | findstr :4000 >nul 2>nul
if %errorlevel% equ 0 (
    echo [WARN] Port 4000 is already in use (backend)
    set /p choice="Do you want to continue anyway? (y/N): "
    if /i not "!choice!"=="y" (
        echo [INFO] E2E setup cancelled
        exit /b 1
    )
)

echo [INFO] Port check completed

REM Start backend server in background
echo [INFO] Starting backend server...
start /B cmd /C "cd backend && npm run start:dev"

REM Wait for backend to be ready
echo [INFO] Waiting for backend server to be ready...
timeout /t 5 /nobreak >nul
powershell -Command "try { Invoke-WebRequest -Uri http://localhost:4000/health -TimeoutSec 1 | Out-Null; Write-Host '[INFO] Backend server is ready' } catch { Write-Host '[INFO] Waiting for backend...' }"

:wait_backend
powershell -Command "try { Invoke-WebRequest -Uri http://localhost:4000/health -TimeoutSec 1 | Out-Null; Write-Host '[INFO] Backend server is ready' } catch { exit 1 }"
if %errorlevel% neq 0 (
    timeout /t 2 /nobreak >nul
    goto wait_backend
)

REM Start frontend server in background
echo [INFO] Starting frontend server...
start /B cmd /C "cd frontend && npm run dev"

REM Wait for frontend to be ready
echo [INFO] Waiting for frontend server to be ready...
timeout /t 5 /nobreak >nul

:wait_frontend
powershell -Command "try { Invoke-WebRequest -Uri http://localhost:3000 -TimeoutSec 1 | Out-Null; Write-Host '[INFO] Frontend server is ready' } catch { exit 1 }"
if %errorlevel% neq 0 (
    timeout /t 2 /nobreak >nul
    goto wait_frontend
)

echo [INFO] Both servers are running:
echo [INFO]   - Frontend: http://localhost:3000
echo [INFO]   - Backend: http://localhost:4000

REM Run e2e tests
echo [INFO] Running E2E tests...
npm run test:e2e

echo [INFO] E2E test setup completed successfully!

REM Note: Servers will continue running in background
echo [INFO] To stop the servers, you may need to manually close the command windows that were opened