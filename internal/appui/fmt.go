package appui

import (
	"fmt"
	"strconv"
)

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
