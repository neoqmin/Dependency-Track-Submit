; NSIS installer for dtrack-submit (CLI + MCP server) — Windows x64.
;
; Expects, in the same directory as this script at build time:
;   dtrack-submit.exe         (renamed from dtrack-submit-windows-amd64.exe)
;   dtrack-mcp-server.exe     (renamed from dtrack-mcp-server-windows-amd64.exe)
;
; VERSION is passed by the workflow: makensis -DVERSION=1.2.3 dtrack-submit.nsi
; Requires the EnVar plugin (https://nsis.sourceforge.io/EnVar_plug-in) for PATH editing.

!ifndef VERSION
  !define VERSION "0.0.0"
!endif

Name "dtrack-submit ${VERSION}"
OutFile "dtrack-submit-setup-${VERSION}.exe"
Unicode True
; Per-user install: the MCP server is launched non-elevated by the MCP client,
; and self-update must be able to overwrite the binary in place. A Program Files
; install would make dtrack_check_update fail with Access Denied. $LOCALAPPDATA
; is user-writable and needs no UAC prompt.
InstallDir "$LOCALAPPDATA\Programs\dtrack-submit"
InstallDirRegKey HKCU "Software\dtrack-submit" "InstallDir"
RequestExecutionLevel user

!include "MUI2.nsh"

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Korean"

Section "dtrack-submit" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "dtrack-submit.exe"
  File "dtrack-mcp-server.exe"

  ; Add install dir to the per-user PATH (idempotent — EnVar checks for dupes).
  EnVar::SetHKCU
  EnVar::AddValue "PATH" "$INSTDIR"
  Pop $0

  WriteRegStr HKCU "Software\dtrack-submit" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\dtrack-submit" "Version" "${VERSION}"

  ; Add/Remove Programs entry (per-user).
  !define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\dtrack-submit"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "dtrack-submit"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "pribit"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1

  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\dtrack-submit.exe"
  Delete "$INSTDIR\dtrack-mcp-server.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  EnVar::SetHKCU
  EnVar::DeleteValue "PATH" "$INSTDIR"
  Pop $0

  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\dtrack-submit"
  DeleteRegKey HKCU "Software\dtrack-submit"
SectionEnd
