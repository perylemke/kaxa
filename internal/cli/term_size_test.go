package cli

import "testing"

func TestTermRowsColsClamps(t *testing.T) {
	t.Parallel()

	if got := termRows(0); got != 24 {
		t.Fatalf("rows(0) = %d", got)
	}
	if got := termCols(0); got != 80 {
		t.Fatalf("cols(0) = %d", got)
	}
	const huge = int(^uint(0) >> 1)
	if termRows(huge) != ^uint16(0) || termCols(huge) != ^uint16(0) {
		t.Fatal("expected clamp to max uint16")
	}
}
