package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestMainExecutesCLI(t *testing.T) {
	if os.Getenv("DEVDB_TEST_MAIN") == "1" {
		os.Args = []string{"devdb", "help"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExecutesCLI$")
	cmd.Env = append(os.Environ(), "DEVDB_TEST_MAIN=1")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
