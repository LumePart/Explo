package util

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
)

func ExecUtility(workdir string, name string, args ...string) ([]byte, error) {
	var out bytes.Buffer
	var errout bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	cmd.Stdout = &out
	cmd.Stderr = &errout
	err := cmd.Run()

	if err != nil {
		log.Println(errout.String())
		return nil, fmt.Errorf("%s launch error: %v", name, err)
	}

	return out.Bytes(), nil
}
