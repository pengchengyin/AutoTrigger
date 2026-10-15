@echo off
rem 由 InteractiveToken 计划任务调用，把 AutoTrigger 桌面端以管理员身份拉起到 Console 会话。
rem 用法与原理见同目录 README.md。
cscript //nologo "%~dp0start-desktop-elevated.vbs"
exit /b 0
