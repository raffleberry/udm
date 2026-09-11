package host

import (
	"os"
	"strings"
)

func IsGoRun() bool {
	execPath := os.Args[0]
	return strings.Contains(execPath, "go-build")
}
