; NetSwitcher NSIS installer (spec §7 Phase 7).
; Build with:  makensis build/windows/installer.nsi
; (run from repo root after `make build` produced netswitcher.exe).
;
; Requirements: NSIS 3.x. WebView2 EverBootstrapper is optional — bundle
; MicrosoftEdgeWebView2Setup.exe next to this script for Win10 fallback.

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "WinVer.nsh"

Name "NetSwitcher"
OutFile "..\..\dist\NetSwitcher-Setup.exe"
Unicode True
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma
RequestExecutionLevel admin

InstallDir "$PROGRAMFILES64\NetSwitcher"
InstallDirRegKey HKLM "Software\NetSwitcher" "InstallDir"

VIProductVersion "0.1.0.0"
VIAddVersionKey "ProductName" "NetSwitcher"
VIAddVersionKey "CompanyName" "NetSwitcher"
VIAddVersionKey "LegalCopyright" "Copyright (c) 2026 NetSwitcher"
VIAddVersionKey "FileDescription" "NetSwitcher Installer"
VIAddVersionKey "FileVersion" "0.1.0"
VIAddVersionKey "ProductVersion" "0.1.0"

; ---- Modern UI ----
!define MUI_ABORTWARNING
!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_RESERVEFILE_LANGDLL

Section "NetSwitcher (required)" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "..\..\netswitcher.exe"
  File /nonfatal "icon.ico"

  ; Write uninstaller + registry entries.
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "Software\NetSwitcher" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NetSwitcher" \
                 "DisplayName" "NetSwitcher"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NetSwitcher" \
                 "UninstallString" "$\"$INSTDIR\Uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NetSwitcher" \
                 "DisplayVersion" "0.1.0"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NetSwitcher" \
                 "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NetSwitcher" \
                 "NoRepair" 1

  ; Shortcuts (GUI). Start menu + desktop.
  CreateDirectory "$SMPROGRAMS\NetSwitcher"
  CreateShortCut "$SMPROGRAMS\NetSwitcher\NetSwitcher.lnk" "$INSTDIR\netswitcher.exe" "gui" "$INSTDIR\icon.ico"
  CreateShortCut "$SMPROGRAMS\NetSwitcher\卸载 NetSwitcher.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortCut "$DESKTOP\NetSwitcher.lnk" "$INSTDIR\netswitcher.exe" "gui" "$INSTDIR\icon.ico"

  ; WebView2 EverBootstrapper (bundled file is optional; no-op if runtime
  ; already present). Needed for older Win10 builds without WebView2.
  IfFileExists "$INSTDIR\MicrosoftEdgeWebview2Setup.exe" 0 skipWebview
    DetailPrint "Ensuring WebView2 runtime..."
    nsExec::ExecToLog '$INSTDIR\MicrosoftEdgeWebview2Setup.exe /silent /install'
  skipWebview:

  ; Install + start the service (the installer is already elevated).
  DetailPrint "Installing NetSwitcher service..."
  nsExec::ExecToLog '"$INSTDIR\netswitcher.exe" service install'
  DetailPrint "Starting NetSwitcher service..."
  nsExec::ExecToLog '"$INSTDIR\netswitcher.exe" service start'
SectionEnd

Section "Uninstall"
  ; Stop + remove the service first (best effort).
  DetailPrint "Stopping NetSwitcher service..."
  nsExec::ExecToLog '"$INSTDIR\netswitcher.exe" service stop'
  DetailPrint "Removing NetSwitcher service..."
  nsExec::ExecToLog '"$INSTDIR\netswitcher.exe" service uninstall'

  Delete "$INSTDIR\netswitcher.exe"
  Delete "$INSTDIR\icon.ico"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\NetSwitcher\NetSwitcher.lnk"
  Delete "$SMPROGRAMS\NetSwitcher\卸载 NetSwitcher.lnk"
  RMDir "$SMPROGRAMS\NetSwitcher"
  Delete "$DESKTOP\NetSwitcher.lnk"

  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\NetSwitcher"
  DeleteRegKey HKLM "Software\NetSwitcher"

  ; Ask whether to remove configuration + state (default: keep).
  MessageBox MB_YESNO|MB_DEFBUTTON2 "同时删除配置与状态 ($PROGRAMDATA\NetSwitcher)？" IDNO end
    RMDir /r "$PROGRAMDATA\NetSwitcher"
  end:
SectionEnd
