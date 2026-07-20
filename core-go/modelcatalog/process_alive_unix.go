//go:build !windows

package modelcatalog

import "syscall"

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
