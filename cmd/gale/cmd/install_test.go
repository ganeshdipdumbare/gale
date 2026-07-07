package cmd

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestResolveInstallResultReturnsCompletedResult covers the common path:
// the install goroutine already finished (or finishes concurrently) by the
// time the TUI exits, so its result should be returned as-is without ever
// cancelling the context.
func TestResolveInstallResultReturnsCompletedResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	installResult := make(chan error, 1)
	wantErr := errors.New("boom")
	installResult <- wantErr

	got := resolveInstallResult(cancel, installResult)
	if !errors.Is(got, wantErr) {
		t.Errorf("expected the install goroutine's own result to be returned, got %v", got)
	}
	if ctx.Err() != nil {
		t.Error("cancel should not be invoked when the install already finished")
	}
}

func TestResolveInstallResultReturnsNilOnSuccess(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	installResult := make(chan error, 1)
	installResult <- nil

	if got := resolveInstallResult(cancel, installResult); got != nil {
		t.Errorf("expected nil error on a successful install, got %v", got)
	}
}

// TestResolveInstallResultCancelsAndWaitsOnEarlyQuit is a regression test
// for the data race originally flagged in code review: reading a plain
// `installErr` shared variable after prog.Run() returns raced with the
// install goroutine's write when the TUI was quit before the install
// finished. This exercises exactly that timing — resolveInstallResult is
// called before the goroutine has produced a result — and verifies it (a)
// cancels the context so the install stops, (b) blocks until the goroutine
// actually finishes rather than returning a stale/zero value, and (c)
// returns a clear cancellation error. Run with -race to confirm there is no
// data race.
func TestResolveInstallResultCancelsAndWaitsOnEarlyQuit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	installResult := make(chan error, 1)
	goroutineStarted := make(chan struct{})
	goroutineFinished := make(chan struct{})

	// Simulates the real install goroutine in install.go: it keeps "running"
	// until ctx is cancelled, only then does it write its result.
	go func() {
		close(goroutineStarted)
		<-ctx.Done()
		// Simulate a little cleanup work happening after cancellation, so
		// the test can prove resolveInstallResult actually waits for it
		// instead of returning early.
		time.Sleep(20 * time.Millisecond)
		installResult <- ctx.Err()
		close(goroutineFinished)
	}()

	<-goroutineStarted // ensure resolveInstallResult is called before any result exists

	got := resolveInstallResult(cancel, installResult)

	select {
	case <-goroutineFinished:
	default:
		t.Error("resolveInstallResult returned before the install goroutine finished")
	}
	if ctx.Err() == nil {
		t.Error("expected resolveInstallResult to cancel the context on early quit")
	}
	if got == nil || got.Error() != "install cancelled" {
		t.Errorf("expected a clear cancellation error, got %v", got)
	}
}
