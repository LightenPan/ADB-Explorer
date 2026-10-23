//go:build linux

package hostswrite

import (
	"os"
	"os/exec"
	"strings"
)

// writePrivileged asks pkexec for the write, or sudo -n when pkexec is not
// installed, as the desktop's own authentication dialog owns the prompt.
func writePrivileged(path string, data []byte) error {
	stage, err := stageContent(data)
	if err != nil {
		return err
	}
	defer os.Remove(stage)
	candidates := [][]string{
		{"pkexec", "cp", stage, path},
		{"sudo", "-n", "cp", stage, path},
	}
	var last string
	for _, args := range candidates {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		if _, lookErr := exec.LookPath(args[0]); lookErr != nil {
			continue
		}
		last = strings.TrimSpace(string(out))
		if strings.Contains(last, "Not authorized") || strings.Contains(last, "dismissed") {
			return &ApplyError{Code: "cancelled", Message: "the authentication prompt was cancelled"}
		}
	}
	if last == "" {
		last = "no privileged helper available (install pkexec or configure sudo)"
	}
	return &ApplyError{Code: "no_access", Message: "writing the hosts file failed: " + last}
}
