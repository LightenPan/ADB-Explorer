package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// nsiScript builds the NSIS script: a per-machine install in Program Files,
// which the directory page still lets the user change, with Start menu and
// desktop shortcuts and an uninstaller that Settings > Apps lists.
func nsiScript(o Options, files []string) string {
	var fileLines strings.Builder
	for _, name := range files {
		// NSIS reads /oname= up to the next space, and takes no quotes
		// there: the installed names have no spaces.
		fmt.Fprintf(&fileLines, "  File /oname=%s %s\n",
			name, nsiString(filepath.Join(mustAbs(o.Stage), name)))
	}

	iconDefines := ""
	if o.Icon != "" {
		iconDefines = "!define MUI_ICON " + nsiString(mustAbs(o.Icon)) + "\n" +
			"!define MUI_UNICON " + nsiString(mustAbs(o.Icon)) + "\n"
	}
	shortcut := nsiEscape(o.Name) + ".lnk"
	uninstallKey := `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + o.Identifier
	openCommand := `'"$INSTDIR\` + nsiEscape(o.Executable) + `"'`

	script := `Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
; The app's own manifest already asks for administrator rights; the
; installer needs them to write Program Files.
RequestExecutionLevel admin
Name ` + nsiString(o.Name) + `
BrandingText ` + nsiString(o.Name+" "+o.Version) + `
OutFile ` + nsiString(mustAbs(o.Out)) + `

; Program Files, not a folder of the user's profile: the install belongs to
; the machine, and every account runs the same copy.
InstallDir "$PROGRAMFILES64\` + nsiEscape(o.Name) + `"
InstallDirRegKey HKLM ` + nsiString(`Software\`+o.Identifier) + ` "InstallDir"

` + iconDefines + `
!include "MUI2.nsh"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\` + nsiEscape(o.Executable) + `"
!define MUI_FINISHPAGE_RUN_TEXT ` + nsiString("Launch "+o.Name) + `
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Create a desktop shortcut"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  ; A running copy cannot be replaced.
  FindWindow $0 "` + nsiEscape(windowClass(o.Name)) + `"
  StrCmp $0 0 done
  MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "` + nsiEscape("Please quit "+o.Name+" and click OK to continue.") + `" IDOK done
  Abort
  done:
FunctionEnd

Function CreateDesktopShortcut
  SetShellVarContext all
  CreateShortCut "$DESKTOP\` + shortcut + `" "$INSTDIR\` + nsiEscape(o.Executable) + `"
FunctionEnd

Section
  SetShellVarContext all
  ; Write the 64-bit view of the registry, where a 32-bit installer would
  ; otherwise be redirected to WOW6432Node, hiding the app from Settings.
  SetRegView 64
  SetOutPath "$INSTDIR"
` + fileLines.String() + `  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateDirectory "$SMPROGRAMS\` + nsiEscape(o.Name) + `"
  CreateShortCut "$SMPROGRAMS\` + nsiEscape(o.Name) + `\` + shortcut + `" "$INSTDIR\` + nsiEscape(o.Executable) + `"
  CreateShortCut "$SMPROGRAMS\` + nsiEscape(o.Name) + `\Uninstall.lnk" "$INSTDIR\Uninstall.exe"

  WriteRegStr HKLM ` + nsiString(`Software\`+o.Identifier) + ` "InstallDir" "$INSTDIR"
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "DisplayName" ` + nsiString(o.Name) + `
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "DisplayVersion" ` + nsiString(o.Version) + `
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "DisplayIcon" "$INSTDIR\` + nsiEscape(o.Executable) + `"
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "Publisher" ` + nsiString(o.Publisher) + `
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "URLInfoAbout" ` + nsiString(o.URL) + `
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKLM ` + nsiString(uninstallKey) + ` "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKLM ` + nsiString(uninstallKey) + ` "NoModify" 1
  WriteRegDWORD HKLM ` + nsiString(uninstallKey) + ` "NoRepair" 1
  WriteRegDWORD HKLM ` + nsiString(uninstallKey) + ` "EstimatedSize" ` + fmt.Sprint(estimatedKB(o.Stage, files)) + `

  ; Let Explorer redraw the new icons and shortcuts.
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
SectionEnd

Function un.onInit
  SetShellVarContext all
  SetRegView 64
  FindWindow $0 "` + nsiEscape(windowClass(o.Name)) + `"
  StrCmp $0 0 done
  MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "` + nsiEscape("Please quit "+o.Name+" and click OK to continue.") + `" IDOK done
  Abort
  done:
FunctionEnd

Section "Uninstall"
  SetShellVarContext all
  SetRegView 64
  ; The uninstaller runs from a copy of itself, so the original is free to
  ; delete; the folder goes once it is.
  Delete "$INSTDIR\Uninstall.exe"
  ; Shortcuts first; Explorer may hold a new one open for a few seconds.
  Delete "$DESKTOP\` + shortcut + `"
  Delete "$SMPROGRAMS\` + nsiEscape(o.Name) + `\` + shortcut + `"
  Delete "$SMPROGRAMS\` + nsiEscape(o.Name) + `\Uninstall.lnk"
  RMDir "$SMPROGRAMS\` + nsiEscape(o.Name) + `"

  DeleteRegKey HKLM ` + nsiString(uninstallKey) + `
  DeleteRegKey HKLM ` + nsiString(`Software\`+o.Identifier) + `

` + installedDeleteLines(files) + `
  ; The app's data in the user's profile is left alone. The folder itself
  ; goes only once nothing holds it: Windows keeps the uninstaller's own
  ; image open for a moment after it exits.
  ClearErrors
  RMDir "$INSTDIR"
  IfErrors 0 done
  RMDir /r "$INSTDIR"
  done:
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
SectionEnd
`
	_ = openCommand
	return script
}

// installedDeleteLines removes what the installer wrote, at uninstall.
func installedDeleteLines(files []string) string {
	var b strings.Builder
	for _, name := range files {
		fmt.Fprintf(&b, "  Delete %s\n", nsiString(`$INSTDIR\`+name))
	}
	return b.String()
}

// estimatedKB is the installer's EstimatedSize, in kilobytes, from the app's
// files.
func estimatedKB(stage string, files []string) int {
	total := 0
	for _, name := range files {
		if info, err := os.Stat(filepath.Join(stage, name)); err == nil {
			total += int(info.Size())
		}
	}
	return total / 1024
}

// windowClass is the class of the app's window, which the installer looks
// for to know the app is running. Win32 windows of a Go program use the
// default class the window library registers.
func windowClass(name string) string { return name }

// nsiString quotes text for NSIS.
func nsiString(s string) string { return `"` + nsiEscape(s) + `"` }

// absPath makes a path absolute, so that makensis, which compiles from the
// app's directory, finds it.
func absPath(p string) (string, error) { return filepath.Abs(p) }

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// nsiEscape escapes text for an NSIS string in double quotes.
func nsiEscape(s string) string {
	return strings.NewReplacer(`$`, `$$`, `"`, `$\"`, "\n", `$\n`, "\r", `$\r`, "\t", `$\t`).Replace(s)
}
