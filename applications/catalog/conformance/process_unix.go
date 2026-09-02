// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	defaultProcessGrace     = time.Second
	defaultProcessWaitDelay = time.Second
)

type managedProcess struct {
	cmd      *exec.Cmd
	done     chan struct{}
	waitErr  error
	stopOnce sync.Once
	outcome  terminationOutcome
}

type terminationOutcome struct {
	Forced bool
	Err    error
}

func (o terminationOutcome) String() string {
	if o.Err != nil {
		return fmt.Sprintf("termination_error=%v", o.Err)
	}
	if o.Forced {
		return "signal=SIGKILL"
	}
	return "signal=SIGTERM"
}

func startManagedProcess(cmd *exec.Cmd) (*managedProcess, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = defaultProcessWaitDelay
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	process := &managedProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		process.waitErr = cmd.Wait()
		close(process.done)
	}()
	return process, nil
}

func (p *managedProcess) waitFor(timeout time.Duration) bool {
	if timeout <= 0 {
		<-p.done
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.done:
		return true
	case <-timer.C:
		return false
	}
}

func (p *managedProcess) err() error {
	<-p.done
	return p.waitErr
}

func (p *managedProcess) terminate(grace time.Duration) terminationOutcome {
	if grace <= 0 {
		grace = defaultProcessGrace
	}
	p.stopOnce.Do(func() {
		termErr := signalProcessGroup(p.cmd.Process.Pid, syscall.SIGTERM)
		timer := time.NewTimer(grace)
		select {
		case <-p.done:
			timer.Stop()
		case <-timer.C:
			p.outcome.Forced = true
		}
		killErr := signalProcessGroup(p.cmd.Process.Pid, syscall.SIGKILL)
		p.outcome.Err = errors.Join(termErr, killErr)
		<-p.done
	})
	return p.outcome
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
