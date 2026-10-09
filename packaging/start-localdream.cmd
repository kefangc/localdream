@echo off
setlocal
cd /d "%~dp0"

if exist "config.local.cmd" call "config.local.cmd"

echo.
echo Starting Local Dream at http://127.0.0.1:8790
echo Press Ctrl+C in this window to stop it.
echo.
localdream.exe
