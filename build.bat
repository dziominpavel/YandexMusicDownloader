@echo off
rem Сборка app.exe (просто запусти двойным кликом).
rem Не путать с release.bat — тот публикует релиз на GitHub.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-windows.ps1"
pause
