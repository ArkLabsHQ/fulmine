package cln

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	dockerCmd       = "docker"
	containerName   = "cln"
	lightningCliCmd = "lightning-cli"
	network         = "--network=regtest"
)

func runCommand(args ...string) (string, error) {
	cmd := exec.Command(dockerCmd, append([]string{"exec", "-i", containerName, lightningCliCmd, network}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running command: %v, output: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}
