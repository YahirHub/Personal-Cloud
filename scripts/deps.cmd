@echo off
setlocal EnableExtensions

for %%I in ("%~dp0..") do set "PROJECT_ROOT=%%~fI"
cd /d "%PROJECT_ROOT%"

echo ==^> Go
call go version
if errorlevel 1 exit /b 1

echo ==^> Normalizando modulos
call go mod tidy
if errorlevel 1 exit /b 1

echo Dependencias verificadas correctamente.
exit /b 0
