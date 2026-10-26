package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// errTaskCancelled marks a run that stopped because a stop was requested. main()
// turns it into a dedicated exit code so the desktop can tell a user cancel
// apart from a provider or workflow failure.
var errTaskCancelled = errors.New("task cancelled")

// cancelledExitCode is the process exit status for a cancelled run.
const cancelledExitCode = 130

// controlCancelCommand is the line the owning desktop writes to the sidecar's
// control channel to request a graceful stop.
const controlCancelCommand = "cancel"

// watchCancellation returns a context that stops the run when the user
// interrupts the process, or when the owning desktop asks it to stop or
// disappears. The returned stop function releases the signal handler.
//
// Signals alone are not enough: a GUI-launched child never receives a console
// interrupt on Windows, and a force-killed desktop cannot signal anything. The
// control channel covers both, and its end-of-file is itself a stop request.
func watchCancellation(controlStdin bool) (context.Context, func()) {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(ctx)
	stop := func() {
		cancel()
		stopSignals()
	}
	if controlStdin {
		go watchControlChannel(os.Stdin, cancel)
	}
	return ctx, stop
}

// watchControlChannel cancels the run when the owner writes the cancel command
// or closes the control channel. A closed channel means the desktop is gone, so
// the work must stop instead of running unattended.
func watchControlChannel(reader io.Reader, cancel context.CancelFunc) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == controlCancelCommand {
			cancel()
			return
		}
	}
	cancel()
}

// cancellationRequested reports whether a run stopped because a cancellation was
// requested rather than failing on its own.
func cancellationRequested(ctx context.Context, err error) bool {
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, errTaskCancelled)
}

// cancelledError is the safe, displayable error for a stopped run.
func cancelledError() error {
	return fmt.Errorf("%w: the task was cancelled before it finished", errTaskCancelled)
}

// stopIfCancelled records a requested stop and returns the cancellation error
// when a failure was caused by a stop rather than by the work itself.
func stopIfCancelled(ctx context.Context, taskDir string, m *Manifest, err error) error {
	if !cancellationRequested(ctx, err) {
		return nil
	}
	markTaskCancelled(taskDir, m)
	return cancelledError()
}

// markTaskCancelledIfRequested records a requested stop for a command that does
// not run the resume pipeline, so a cancel is durable there too.
func markTaskCancelledIfRequested(ctx context.Context, taskDir string, m *Manifest, err error) {
	if cancellationRequested(ctx, err) {
		markTaskCancelled(taskDir, m)
	}
}

// markTaskCancelled records a requested stop. Completed artifacts and finished
// generations are left untouched; a later resume continues from the earliest
// incomplete stage, and any chunk that was mid-flight is reconciled back to
// pending.
func markTaskCancelled(taskDir string, m *Manifest) {
	retryable := true
	m.State = TaskCancelled
	m.LastError = &ErrorInfo{
		Stage:     cancellationErrorCode,
		Message:   "The task was cancelled before it finished.",
		Retryable: retryable,
		At:        nowFunc().UTC().Format(time.RFC3339),
	}
	_ = SaveManifest(taskDir, m)
}
