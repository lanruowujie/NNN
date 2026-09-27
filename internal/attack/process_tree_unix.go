//go:build !windows

package attack

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
)

type processTree struct {
	cmd  *exec.Cmd
	once sync.Once
}

func prepareProcessTree(cmd *exec.Cmd) (*processTree, error) {
	tree := &processTree{cmd: cmd}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = tree.Kill
	return tree, nil
}

func (*processTree) Started(*exec.Cmd) error { return nil }

func (t *processTree) Kill() error {
	var killErr error
	t.once.Do(func() {
		if t.cmd.Process == nil {
			return
		}
		killErr = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(killErr, syscall.ESRCH) {
			killErr = nil
		}
	})
	return killErr
}

func (t *processTree) Close() error { return t.Kill() }
