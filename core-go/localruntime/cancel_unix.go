//go:build !windows

package localruntime

import (
	"os"
	"os/exec"
	"time"
)

func configureCancellation(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		return command.Process.Signal(os.Interrupt)
	}
	command.WaitDelay = 2 * time.Second
}
