@echo off
REM vibe-local Windows launcher wrapper
REM Launches vibe-local.ps1 with ExecutionPolicy bypass
powershell.exe -ExecutionPolicy Bypass -File "%~dp0vibe-local.ps1" %*
