//go:build !windows && !darwin && !linux

package hostswrite

func CanElevate() bool { return false }

func elevated() bool { return false }

func RelaunchElevated() error {
	return &ApplyError{Code: "no_access", Message: "this platform has no way to ask for administrator rights"}
}
