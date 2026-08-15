@echo off
setlocal EnableExtensions

for %%I in ("%~dp0..") do set "PROJECT_ROOT=%%~fI"
cd /d "%PROJECT_ROOT%"

echo ==^> gofmt
set "UNFORMATTED="
for /f "delims=" %%F in ('gofmt -l ./cmd ./internal ./web') do (
    echo %%F
    set "UNFORMATTED=1"
)
if defined UNFORMATTED (
    echo ERROR: Hay archivos Go sin formato.
    exit /b 1
)

echo ==^> go test
call go test ./...
if errorlevel 1 exit /b 1

echo ==^> go vet
call go vet ./...
if errorlevel 1 exit /b 1

echo OK
exit /b 0
