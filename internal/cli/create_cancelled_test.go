package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func TestCancelledOrErrPrintsMessage(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	err := cancelledOrErr(cmd, errCreateCancelled)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := out.String(); got != "create cancelled\n" {
		t.Fatalf("out = %q", got)
	}
	if err := cancelledOrErr(cmd, errors.New("other")); err == nil || err.Error() != "other" {
		t.Fatalf("err = %v", err)
	}
}
