package hostswrite

import (
	"os"
	"path/filepath"
	"strings"
)

// ElevationHelperArg relaunches the app to write the system hosts file with
// administrator rights on Windows.
const ElevationHelperArg = "--swh-elevated-apply-hosts"

// IsElevationHelper reports whether this process was started to perform a
// privileged write, and carries out the write when it was. It returns true
// when the process should exit with the given code.
func IsElevationHelper(args []string) (bool, int) {
	for i, arg := range args {
		if arg != ElevationHelperArg {
			continue
		}
		if i+1 >= len(args) {
			return true, 1
		}
		src := args[i+1]
		data, err := os.ReadFile(src)
		if err != nil {
			return true, 1
		}
		target, err := SystemHostsPath()
		if err != nil {
			return true, 1
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return true, 1
		}
		_ = os.Remove(src)
		return true, 0
	}
	return false, 0
}

// writeFile writes the system hosts file with platform-native permissions.
func writeFile(path string, data []byte) error {
	force, err := os.ReadFile(path)
	if err == nil && string(force) == string(data) {
		return nil
	}
	// Preserve the file's permissions when it exists.
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(path, data, perm); err == nil {
		return nil
	}
	return writePrivileged(path, data)
}

// stageContent writes data to a temporary file for a privileged writer.
func stageContent(data []byte) (string, error) {
	f, err := os.CreateTemp("", "swh_apply_*.hosts")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// selfPath is the running executable, used to relaunch elevated.
func selfPath() (string, error) {
	return os.Executable()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func baseName(path string) string { return filepath.Base(path) }
