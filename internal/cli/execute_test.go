package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func resetFlags() {
	flagRepo = ""
	flagDB = ""
	flagAll = false
	flagVerbose = false
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	resetFlags()

	oldArgs := os.Args
	os.Args = append([]string{"devdb"}, args...)
	t.Cleanup(func() { os.Args = oldArgs })

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	outDone := make(chan string)
	errDone := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outDone <- buf.String()
	}()
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errDone <- buf.String()
	}()

	code = Execute()

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return <-outDone, <-errDone, code
}
