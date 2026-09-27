//go:build windows

package attack

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processTree struct {
	cmd  *exec.Cmd
	job  windows.Handle
	once sync.Once
}

func prepareProcessTree(cmd *exec.Cmd) (*processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&information)), uint32(unsafe.Sizeof(information)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	tree := &processTree{cmd: cmd, job: job}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = tree.Kill
	return tree, nil
}

func (t *processTree) Started(cmd *exec.Cmd) error {
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		// A very short version probe may exit between Start and OpenProcess.
		// There is no live process tree left to attach in that case.
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.AssignProcessToJobObject(t.job, handle)
}

func (t *processTree) Kill() error {
	var killErr error
	t.once.Do(func() {
		killErr = windows.TerminateJobObject(t.job, 1)
		if t.cmd.Process != nil {
			processErr := t.cmd.Process.Kill()
			if killErr == nil {
				killErr = processErr
			}
		}
	})
	return killErr
}

func (t *processTree) Close() error {
	err := windows.CloseHandle(t.job)
	if errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		return nil
	}
	return err
}
