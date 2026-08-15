@echo off
setlocal EnableExtensions

for %%I in ("%~dp0..") do set "PROJECT_ROOT=%%~fI"
cd /d "%PROJECT_ROOT%"

echo ==^> Go
call go version
if errorlevel 1 exit /b 1

echo ==^> gofmt
set "UNFORMATTED="
for /f "delims=" %%F in ('gofmt -l ./cmd ./internal') do (
    echo %%F
    set "UNFORMATTED=1"
)
if defined UNFORMATTED (
    echo ERROR: Hay archivos Go sin formato.
    exit /b 1
)

echo ==^> go mod tidy
call go mod tidy
if errorlevel 1 exit /b 1

echo ==^> go test
call go test ./...
if errorlevel 1 exit /b 1

echo ==^> go vet
call go vet ./...
if errorlevel 1 exit /b 1

echo ==^> build
call go build ./cmd/server
if errorlevel 1 exit /b 1
if exist server.exe del /q server.exe >nul 2>&1
if exist server del /q server >nul 2>&1

echo OK
exit /b 0
