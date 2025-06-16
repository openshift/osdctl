package iexec

import (
	"fmt"
	"os/exec"
)

// This interface is used to abstract the execution of commands
// It defines a single method `execute` that takes a command and its arguments,
// and returns the output as a string and an error if any occurred.
type IExec interface {
	Run(dir string, name string, args ...string) error
	Output(dir, cmd string, args ...string) (string, error)
	CombinedOutput(dir, cmd string, args ...string) (string, error)
}

type Exec struct {
}

func (e Exec) Run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %v", err)
	}
	return nil
}

func (e Exec) Output(dir, cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)
	// if dir is "" then use the current directory as per https://pkg.go.dev/os/exec#Cmd.Dir
	command.Dir = dir
	out, err := command.Output()
	return string(out), err
}

func (e Exec) CombinedOutput(dir, cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	return string(out), err
}
