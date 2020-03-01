package buildah

import (
	"bytes"
	"io"
	"os/exec"
)

// Execute shell command through informed though a slice of strings. It can return error in case of
// the command itself returning error, where byte output should reflect error message.
func Execute(cmdSlice []string, stdin io.Reader, verbose bool) ([]byte, error) {
	log.V(3).Infof("Executing shell command '%s'", cmdSlice)

	var cmd *exec.Cmd
	if len(cmdSlice) == 1 {
		cmd = exec.Command(cmdSlice[0], []string{}...)
	} else {
		cmd = exec.Command(cmdSlice[0], cmdSlice[1:]...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = stdin

	if err := cmd.Run(); err != nil {
		if verbose {
			log.V(0).Infof("ERROR: Command '%q' failed with error '%s', stdout: '%s', stderr: '%s'",
				cmdSlice, err, stdout.Bytes(), stderr.Bytes())
		}
		return nil, err
	}
	if verbose {
		log.V(5).Infof("command='%q', stdout='%s', stderr='%s'",
			cmdSlice, stdout.Bytes(), stderr.Bytes())
	}
	return stdout.Bytes(), nil
}
