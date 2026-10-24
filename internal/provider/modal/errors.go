package modal

import (
	"fmt"

	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

const notAvailableHint = "this provider is not available yet"

func errNotImplemented(op string) error {
	return usererr.Wrap(
		sandbox.ErrNotImplemented,
		fmt.Sprintf("modal %s is not implemented", op),
		notAvailableHint,
	)
}
