//go:build linux

package hostswrite

import (
	"os"
	"os/exec"
	"strings"
)

// CanElevate reports whether the desktop offers a way to ask for
// administrator rights.
func CanElevate() bool {
	if _, err := exec.LookPath("pkexec"); err == nil {
		return true
	}
	_, err := exec.LookPath("sudo")
	return err == nil
}

// elevated reports whether the process holds root rights.
func elevated() bool { return os.Geteuid() == 0 }

// RelaunchElevated restarts the app through pkexec, or sudo in a terminal,
// so the new instance holds the rights the prompt grants.
func RelaunchElevated() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	attempts := [][]string{{"pkexec", exe}, {"sudo", "-n", exe}}
	var last string
	for _, args := range attempts {
		if _, lookErr := exec.LookPath(args[0]); lookErr != nil {
			continue
		}
		cmd := exec.Command(args[0], args[1:]...)
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return nil
		}
		last = strings.TrimSpace(string(out))
		if strings.Contains(last, "Not authorized") || strings.Contains(last, "dismissed") {
			return &ApplyError{Code: "cancelled", Message: "the authentication prompt was cancelled"}
		}
	}
	if last == "" {
		last = "no pkexec or sudo available"
	}
	return &ApplyError{Code: "no_access", Message: "could not restart with administrator rights: " + last}
}
