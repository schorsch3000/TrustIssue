@echo off
rem Builds trustctl-tray.exe with the C# compiler that comes with Windows (.NET Framework 4, no SDK needed).
setlocal
set CSC=%WINDIR%\Microsoft.NET\Framework64\v4.0.30319\csc.exe
if not exist "%CSC%" set CSC=%WINDIR%\Microsoft.NET\Framework\v4.0.30319\csc.exe
"%CSC%" /nologo /target:winexe /optimize+ /out:"%~dp0trustctl-tray.exe" /reference:System.Windows.Forms.dll /reference:System.Drawing.dll "%~dp0TrustctlTray.cs"
if errorlevel 1 exit /b 1
echo built %~dp0trustctl-tray.exe
