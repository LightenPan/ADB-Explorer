//go:build windows

package hostswrite

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")

	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	procWait     = kernel32.NewProc("WaitForSingleObject")
	procExitCode = kernel32.NewProc("GetExitCodeProcess")
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     uintptr
}

const seeMaskNoAsync = 0x00000100
const seeMaskNoCloseProcess = 0x00000040

// writePrivileged relaunches this executable with administrator rights to
// copy staged content over the system hosts file.
func writePrivileged(path string, data []byte) error {
	stage, err := stageContent(data)
	if err != nil {
		return err
	}
	exe, err := selfPath()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(ElevationHelperArg + ` "` + stage + `"`)

	info := shellExecuteInfo{
		fMask:        seeMaskNoAsync,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        0,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	ok, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		_ = os.Remove(stage)
		if callErr == syscall.Errno(1223) { // ERROR_CANCELLED
			return &ApplyError{Code: "cancelled", Message: "the elevation prompt was cancelled"}
		}
		return &ApplyError{Code: "no_access", Message: "could not start the elevation helper: " + callErr.Error()}
	}
	if info.hProcess != 0 {
		_, _, _ = procWait.Call(info.hProcess, 120000)
		var code uint32
		_, _, _ = procExitCode.Call(info.hProcess, uintptr(unsafe.Pointer(&code)))
		syscall.CloseHandle(syscall.Handle(info.hProcess))
		_ = os.Remove(stage)
		if code != 0 {
			return &ApplyError{Code: "no_access", Message: fmt.Sprintf("the elevation helper failed (%d)", code)}
		}
		return nil
	}
	_ = os.Remove(stage)
	return nil
}

var errUnsupported = errors.New("unsupported platform")
