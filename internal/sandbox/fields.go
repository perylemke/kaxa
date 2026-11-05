package sandbox

import (
	"fmt"
	"strconv"
	"time"
)

// AppendTextField appends a field when text is non-empty.
func AppendTextField(fields []Field, label, text string) []Field {
	if text == "" {
		return fields
	}
	return append(fields, Field{Label: label, Text: text})
}

// AppendTimeField appends a field when t is non-zero.
func AppendTimeField(fields []Field, label string, t time.Time) []Field {
	if t.IsZero() {
		return fields
	}
	return append(fields, Field{Label: label, At: t})
}

// AppendIntField appends a field when n is non-zero.
func AppendIntField(fields []Field, label string, n int) []Field {
	if n == 0 {
		return fields
	}
	return append(fields, Field{Label: label, Text: strconv.Itoa(n)})
}

// AppendInt64Field appends a field when n is non-zero.
func AppendInt64Field(fields []Field, label string, n int64) []Field {
	if n == 0 {
		return fields
	}
	return append(fields, Field{Label: label, Text: strconv.FormatInt(n, 10)})
}

// AppendBoolField appends a field when b is true.
func AppendBoolField(fields []Field, label string, b bool) []Field {
	if !b {
		return fields
	}
	return append(fields, Field{Label: label, Text: "true"})
}

// AppendOptionalTimeField appends a field when t is non-nil and non-zero.
func AppendOptionalTimeField(fields []Field, label string, t *time.Time) []Field {
	if t == nil || t.IsZero() {
		return fields
	}
	return AppendTimeField(fields, label, *t)
}

// FormatFieldValue returns the text shown for one field.
func FormatFieldValue(f Field) string {
	if !f.At.IsZero() {
		return f.At.Format(time.RFC3339)
	}
	return f.Text
}

// AppendFormattedField appends a field when the formatted value is non-empty.
func AppendFormattedField(fields []Field, label, format string, args ...any) []Field {
	return AppendTextField(fields, label, fmt.Sprintf(format, args...))
}
