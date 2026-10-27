package cli

import (
	"context"
	"fmt"
	"io"
)

func runConfirmedSandboxAction(
	ctx context.Context,
	in io.Reader,
	errOut, out io.Writer,
	prompt, spinnerLabel, successTitle, successMsg string,
	action func(context.Context) error,
) error {
	ok, err := confirmAction(in, errOut, prompt)
	if err != nil {
		return err
	}
	if !ok {
		_, err = fmt.Fprintln(out, "cancelled")
		return err
	}
	if err := runTaskWithSpinner(ctx, errOut, spinnerLabel, action); err != nil {
		return err
	}
	return WriteSuccess(out, successTitle, successMsg)
}
