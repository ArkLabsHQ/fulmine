package cln

import (
	"fmt"
	"os/exec"
	"strings"
)

func runCommand(name string, arg ...string) (string, error) {
	var stdout, stderr strings.Builder

	cmd := newCommand(name, arg...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if len(errMsg) > 0 {
			return "", fmt.Errorf("%s", errMsg)
		}

		outMsg := strings.TrimSpace(stdout.String())
		if len(outMsg) > 0 {
			return "", fmt.Errorf("%s", outMsg)
		}

		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

func newCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
