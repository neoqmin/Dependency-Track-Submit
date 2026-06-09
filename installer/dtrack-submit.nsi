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
!include "nsDialogs.nsh"
!include "LogicLib.nsh"

; Variables holding the user-entered Dependency-Track connection settings.
Var Dialog
Var ServerLabel
Var ServerInput
Var ApiKeyLabel
Var ApiKeyInput
Var HintLabel
Var ServerURL
Var ApiKey

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_DIRECTORY
; Custom page to collect server URL + API key, before files are written.
Page custom ConfigPageCreate ConfigPageLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Korean"

; ── Config input page ─────────────────────────────────────────────────────────

Function ConfigPageCreate
  !insertmacro MUI_HEADER_TEXT "Dependency-Track 연결 설정" "서버 주소와 API 키를 입력하세요."

  ; Pre-fill the server URL from a previous install (stored in the registry).
  ; The API key is never stored in the registry; on upgrade, leaving it blank
  ; keeps the existing key in the config files.
  ReadRegStr $ServerURL HKCU "Software\dtrack-submit" "Server"

  nsDialogs::Create 1018
  Pop $Dialog
  ${If} $Dialog == error
    Abort
  ${EndIf}

  ${NSD_CreateLabel} 0 0 100% 12u "Dependency-Track 서버 URL (예: http://localhost:8080)"
  Pop $ServerLabel
  ${NSD_CreateText} 0 14u 100% 12u "$ServerURL"
  Pop $ServerInput

  ${NSD_CreateLabel} 0 34u 100% 12u "API 키 (예: odt_xxxxxxxx)"
  Pop $ApiKeyLabel
  ${NSD_CreatePassword} 0 48u 100% 12u ""
  Pop $ApiKeyInput

  ${NSD_CreateLabel} 0 68u 100% 24u "값은 사용자 홈의 .dtrack.json 및 설치 폴더의 config.json에 저장됩니다.$\r$\n업그레이드 시 빈칸으로 두면 기존 값이 유지됩니다."
  Pop $HintLabel

  nsDialogs::Show
FunctionEnd

Function ConfigPageLeave
  ${NSD_GetText} $ServerInput $ServerURL
  ${NSD_GetText} $ApiKeyInput $ApiKey
FunctionEnd

; ── Helpers ───────────────────────────────────────────────────────────────────

; WriteConfigFile: writes {"server":$ServerURL,"api_key":$ApiKey} to path $9,
; overwriting it. Callers only invoke this when both values are non-blank, so an
; upgrade with a left-blank API key leaves existing config files untouched.
Function WriteConfigFile
  FileOpen $8 "$9" w
  ${If} $8 != ""
    FileWrite $8 '{$\r$\n'
    FileWrite $8 '  "server": "$ServerURL",$\r$\n'
    FileWrite $8 '  "api_key": "$ApiKey"$\r$\n'
    FileWrite $8 '}$\r$\n'
    FileClose $8
  ${EndIf}
FunctionEnd

Section "dtrack-submit" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "dtrack-submit.exe"
  File "dtrack-mcp-server.exe"

  ; Write config files only when BOTH server and API key are provided. This
  ; avoids clobbering a working config with blanks on a click-through upgrade
  ; (leave the API key blank to keep existing config files as-is).
  ${If} $ServerURL != ""
  ${AndIf} $ApiKey != ""
    ; MCP server reads ~/.dtrack.json; CLI reads config.json next to the exe.
    StrCpy $9 "$PROFILE\.dtrack.json"
    Call WriteConfigFile
    StrCpy $9 "$INSTDIR\config.json"
    Call WriteConfigFile
  ${EndIf}

  ; Persist the (non-secret) server URL for pre-fill on the next upgrade.
  ${If} $ServerURL != ""
    WriteRegStr HKCU "Software\dtrack-submit" "Server" "$ServerURL"
  ${EndIf}

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
  Delete "$INSTDIR\config.json"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  EnVar::SetHKCU
  EnVar::DeleteValue "PATH" "$INSTDIR"
  Pop $0

  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\dtrack-submit"
  DeleteRegKey HKCU "Software\dtrack-submit"
  ; Note: ~/.dtrack.json is left in place (may be shared with the CLI / manual use).
SectionEnd
