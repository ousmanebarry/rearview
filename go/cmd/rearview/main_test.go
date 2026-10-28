package main

import (
	"bytes"
	"testing"
)

// execute runs the CLI with args and returns stdout, stderr and the error.
func execute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := newRootCmd(&out, &errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		out, _, err := execute(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if want := "rearview " + version + "\n"; out != want {
			t.Fatalf("%v: output = %q, want %q", args, out, want)
		}
	}
}
