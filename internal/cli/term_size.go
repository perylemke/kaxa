package cli

import "math"

func termRows(h int) uint16 {
	if h <= 0 {
		return 24
	}
	if h > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(h)
}

func termCols(w int) uint16 {
	if w <= 0 {
		return 80
	}
	if w > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(w)
}
