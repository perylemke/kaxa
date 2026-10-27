package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
)

func TestUnknownProviderErrorRendersStyledBlock(t *testing.T) {
	reg := testRegistry(t)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, "nope", testCmdList})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	out := new(bytes.Buffer)
	if err := cli.WriteError(out, err); err != nil {
		t.Fatalf("WriteError: %v", err)
	}
	plain := out.String()
	if strings.Contains(plain, "error:") {
		t.Fatalf("should not use legacy error prefix: %q", plain)
	}
	if !strings.Contains(plain, "unknown provider") {
		t.Fatalf("output = %q", plain)
	}
}
