; NetSwitcher Windows Installer (NSIS)
; Build:  makensis build\windows\installer.nsi  (from repo root)
; Output: build\bin\NetSwitcher-Setup.exe
; ASCII-only to avoid encoding issues on CJK Windows.

!define APPNAME "NetSwitcher"
!define APPVERSION "0.1.0"
!define WEBVIEW2_REGKEY "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

Unicode true
SetCompressor /SOLID lzma

Name "${APPNAME} ${APPVERSION}"
OutFile "..\bin\NetSwitcher-Setup.exe"
InstallDir "$PROGRAMFILES64\${APPNAME}"
InstallDirRegKey HKLM "Software\${APPNAME}" "InstallDir"
RequestExecutionLevel admin

!include "MUI2.nsh"
!include "FileFunc.nsh"
!define MUI_ABORTWARNING
!define MUI_ICON "..\windows\icon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\netswitcher.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${APPNAME}"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

VIProductVersion "0.1.0.0"
VIAddVersionKey "ProductName" "${APPNAME}"
VIAddVersionKey "FileVersion" "${APPVERSION}"
VIAddVersionKey "ProductVersion" "${APPVERSION}"
VIAddVersionKey "CompanyName" "NetSwitcher"
VIAddVersionKey "FileDescription" "Network route manager"

Section "Install" SecInstall
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "..\..\build\bin\netswitcher.exe"
  File "MicrosoftEdgeWebview2Setup.exe"

  ; --- WebView2 runtime: detect / install ---
  ReadRegStr $0 HKLM "${WEBVIEW2_REGKEY}" "pv"
  StrCmp $0 "" +3 0        ; empty -> install (jump +3 past the next check)
  StrCmp $0 "0.0.0.0" +2 0 ; placeholder -> install
  Goto webview2_done       ; has version -> skip

  DetailPrint "WebView2 not found, installing silently..."
  nsExec::Exec '"$INSTDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
  Pop $R0
  DetailPrint "WebView2 install exit code: $R0"

  webview2_done:
  Delete "$INSTDIR\MicrosoftEdgeWebview2Setup.exe"

  ; --- Data dir ---
  ReadEnvStr $R1 "PROGRAMDATA"
  CreateDirectory "$R1\${APPNAME}\logs"

  ; --- Shortcuts ---
  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortCut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\netswitcher.exe" "" "$INSTDIR\netswitcher.exe" 0
  CreateShortCut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\netswitcher.exe" "" "$INSTDIR\netswitcher.exe" 0
  CreateShortCut "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk" "$INSTDIR\uninstall.exe" "" "" 0

  ; --- Uninstall entry ---
  WriteRegStr HKLM "Software\${APPNAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "DisplayName" "${APPNAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "DisplayIcon" '"$INSTDIR\netswitcher.exe"'
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "DisplayVersion" "${APPVERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "Publisher" "NetSwitcher"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}" "EstimatedSize" "$0"

  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'taskkill /F /IM netswitcher.exe'
  Sleep 1000

  Delete "$INSTDIR\netswitcher.exe"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\MicrosoftEdgeWebview2Setup.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk"
  RMDir "$SMPROGRAMS\${APPNAME}"
  Delete "$DESKTOP\${APPNAME}.lnk"

  nsExec::ExecToLog 'schtasks /Delete /F /TN "${APPNAME}"'

  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}"
  DeleteRegKey HKLM "Software\${APPNAME}"

  ReadEnvStr $R1 "PROGRAMDATA"
  MessageBox MB_YESNO|MB_ICONQUESTION "Also delete configuration and logs?$\n$\n$R1\${APPNAME}\" IDNO +2
    RMDir /r "$R1\${APPNAME}"
SectionEnd
