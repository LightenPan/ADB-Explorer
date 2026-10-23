// Command package-windows builds the Windows installer for the native
// client: a Program Files install and a file name without spaces, instead
// of MyGo's per-user install in %LOCALAPPDATA%\Programs and its
// "<Name> Setup <version> <arch>.exe" name.
//
//	go run ./cmd/package-windows
//
// It expects `mygo build -platform windows/amd64` to have run first, and
// writes build/installers/SwitchHosts-<version>-Setup-x64.exe. The
// installer needs administrator rights to write Program Files, and its
// uninstall entry lives in the 64-bit view of the registry so that
// Settings > Apps lists it.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options are the installer's details.
type Options struct {
	// Name is what the app is called, and the executable's base name.
	Name string
	// Version is the app's version.
	Version string
	// Identifier keys the uninstall entry.
	Identifier string
	// Publisher is the vendor shown by Settings > Apps.
	Publisher string
	// URL is the app's home page.
	URL string
	// Stage is the directory holding the built app files.
	Stage string
	// Executable is the name of the app's executable inside Stage.
	Executable string
	// Icon is a .ico for the installer and its shortcuts.
	Icon string
	// Out is where the installer is written.
	Out string
}

const (
	appName       = "SwitchHosts"
	appVersion    = "5.0.3"
	appIdentifier = "net.oldj.switchhosts"
	appPublisher  = "oldj"
	appHomepage   = "https://switchhosts.app"
)

func main() {
	stage := flag.String("stage", filepath.Join("build", "windows-amd64"), "the built app's directory")
	out := flag.String("out", "", "the installer's path (default: build/installers/<name>-<version>-Setup-x64.exe)")
	flag.Parse()

	opts := Options{
		Name:       appName,
		Version:    appVersion,
		Identifier: appIdentifier,
		Publisher:  appPublisher,
		URL:        appHomepage,
		Stage:      *stage,
		Executable: appName + ".exe",
		Icon:       filepath.Join("assets", "icon.ico"),
	}
	opts.Out = *out
	if opts.Out == "" {
		opts.Out = filepath.Join("build", "installers",
			fmt.Sprintf("%s-%s-Setup-x64.exe", opts.Name, opts.Version))
	}

	if err := writeInstaller(opts); err != nil {
		fmt.Fprintln(os.Stderr, "package-windows:", err)
		os.Exit(1)
	}
}

// writeInstaller compiles the installer.
func writeInstaller(o Options) error {
	makensis, err := findMakensis()
	if err != nil {
		return err
	}
	exe := filepath.Join(o.Stage, o.Executable)
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("%s is missing: run mygo build -platform windows/amd64 first", exe)
	}
	files, err := installedFiles(o.Stage)
	if err != nil {
		return err
	}
	if o.Icon != "" {
		if _, statErr := os.Stat(o.Icon); statErr != nil {
			return fmt.Errorf("installer icon %s: %w", o.Icon, statErr)
		}
	}
	if err := os.MkdirAll(filepath.Dir(o.Out), 0o755); err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "swh-installer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	script := nsiScript(o, files)
	nsi := filepath.Join(work, "installer.nsi")
	// NSIS reads a script without a byte order mark in the machine's code
	// page; the app's paths are UTF-8.
	if err := os.WriteFile(nsi, append([]byte{0xEF, 0xBB, 0xBF}, script...), 0o644); err != nil {
		return err
	}
	cmd := exec.Command(makensis, "-V2", nsi)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("makensis: %w", err)
	}
	fmt.Printf("Created %s\n", o.Out)
	return nil
}

// installedFiles lists what the installer copies: everything mygo build put
// beside the executable, apart from an installer of MyGo's own.
func installedFiles(stage string) ([]string, error) {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".exe") && strings.Contains(lower, "setup") {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// findMakensis locates the NSIS compiler.
func findMakensis() (string, error) {
	if p, err := exec.LookPath("makensis"); err == nil {
		return p, nil
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if dir := os.Getenv(env); dir != "" {
			p := filepath.Join(dir, "NSIS", "makensis.exe")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("NSIS not found: install it from https://nsis.sourceforge.io")
}
