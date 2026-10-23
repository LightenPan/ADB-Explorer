//go:build darwin

package hostswrite

import (
	"os/exec"
	"strings"
)

// CanElevate reports whether the app can ask for administrator rights.
func CanElevate() bool { return true }

// elevated reports whether the process holds administrator rights.
func elevated() bool { return osGeteuid() == 0 }

// RelaunchElevated restarts the app through AppleScript's administrator
// prompt: a new instance inherits the rights the prompt grants.
func RelaunchElevated() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	script := "do shell script " + shellQuote("open -n "+shellQuote(exe)) +
		" with administrator privileges"
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if strings.Contains(message, "-128") || strings.Contains(strings.ToLower(message), "cancel") {
			return &ApplyError{Code: "cancelled", Message: "the administrator prompt was cancelled"}
		}
		return &ApplyError{Code: "no_access", Message: "could not restart with administrator rights: " + message}
	}
	return nil
}
