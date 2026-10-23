//go:build windows

package hostswrite

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	advapi32                 = syscall.NewLazyDLL("advapi32.dll")
	procOpenProcessToken     = advapi32.NewProc("OpenProcessToken")
	procCreateWellKnownSid   = advapi32.NewProc("CreateWellKnownSid")
	procCheckTokenMembership = advapi32.NewProc("CheckTokenMembership")
)

const (
	tokenQuery                  = 0x0008
	winBuiltinAdministratorsSid = 26
	securityMaxSidSize          = 68
)

// CanElevate reports whether the app can restart itself with administrator
// rights, which it can where Windows offers a consent prompt.
func CanElevate() bool { return true }

// elevated reports whether the process holds administrator rights.
func elevated() bool {
	var token syscall.Handle
	proc, _ := syscall.GetCurrentProcess()
	ok, _, _ := procOpenProcessToken.Call(uintptr(proc), tokenQuery, uintptr(unsafe.Pointer(&token)))
	if ok == 0 {
		return false
	}
	defer syscall.CloseHandle(token)

	var sid [securityMaxSidSize]byte
	var size uint32 = securityMaxSidSize
	ok, _, _ = procCreateWellKnownSid.Call(winBuiltinAdministratorsSid, 0,
		uintptr(unsafe.Pointer(&sid[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return false
	}
	var member int32
	ok, _, _ = procCheckTokenMembership.Call(uintptr(token), uintptr(unsafe.Pointer(&sid[0])),
		uintptr(unsafe.Pointer(&member)))
	if ok == 0 {
		return false
	}
	return member != 0
}

// RelaunchElevated starts a new instance with administrator rights and
// returns once it is running; the caller quits this instance.
func RelaunchElevated() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	info := shellExecuteInfo{
		fMask:  seeMaskNoAsync | seeMaskNoCloseProcess,
		lpVerb: verb,
		lpFile: file,
		nShow:  1, // SW_SHOWNORMAL
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	ok, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if callErr == syscall.Errno(1223) { // ERROR_CANCELLED: the prompt was declined
			return &ApplyError{Code: "cancelled", Message: "the administrator consent prompt was declined"}
		}
		return errors.New("could not start the elevated instance: " + callErr.Error())
	}
	if info.hProcess != 0 {
		_ = syscall.CloseHandle(syscall.Handle(info.hProcess))
	}
	return nil
}
