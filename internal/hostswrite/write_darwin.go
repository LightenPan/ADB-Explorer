//go:build darwin

package hostswrite

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// writePrivileged writes through AppleScript's administrator prompt, which
// is how the app has always requested a privileged write on macOS.
func writePrivileged(path string, data []byte) error {
	stage, err := stageContent(data)
	if err != nil {
		return err
	}
	defer os.Remove(stage)
	script := fmt.Sprintf(`do shell script "cat %s > %s" with administrator privileges`, shellQuote(stage), shellQuote(path))
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if strings.Contains(message, "-128") || strings.Contains(strings.ToLower(message), "cancel") {
			return &ApplyError{Code: "cancelled", Message: "the authorization prompt was cancelled"}
		}
		return &ApplyError{Code: "no_access", Message: "writing the hosts file failed: " + message}
	}
	return nil
}
