@echo off
setlocal EnableExtensions

for %%I in ("%~dp0..") do set "PROJECT_ROOT=%%~fI"
cd /d "%PROJECT_ROOT%"

call go run ./cmd/server
exit /b %errorlevel%
