package adb

import (
	"fmt"
	"log"
	"os"
	"time"
)

// UploadLogPath is the trace written while an upload is in progress.
const UploadLogPath = `D:\devel\ai\ADB-Explorer\go\upload.log`

// UploadLog appends one line to stderr and to UploadLogPath.
func UploadLog(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[upload] %s", msg)
	f, err := os.OpenFile(UploadLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), msg)
}
