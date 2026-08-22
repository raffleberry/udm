//go:build !windows

package udm

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}
