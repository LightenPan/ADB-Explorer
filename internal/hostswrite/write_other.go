//go:build !windows && !darwin && !linux

package hostswrite

import "errors"

func writePrivileged(path string, data []byte) error {
	return errors.New("writing the system hosts file is not supported on this platform")
}
