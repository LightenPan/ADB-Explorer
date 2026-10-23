package hostswrite

import "os"

// CanWriteHosts reports whether this process can write the system hosts
// file without asking for elevation, and the reason when it cannot.
func CanWriteHosts() (bool, string) {
	path, err := SystemHostsPath()
	if err != nil {
		return false, err.Error()
	}
	// Opening for writing changes nothing and needs exactly the permission
	// the apply needs.
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false, err.Error()
	}
	_ = file.Close()
	return true, ""
}

// Elevated reports whether this process already runs with administrator
// rights, which the app cannot tell from a write probe when the file is
// writable by the user in its own right.
func Elevated() bool { return elevated() }
