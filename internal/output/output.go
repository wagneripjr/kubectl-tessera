// Package output centralizes the CLI's machine-readable rendering so mint, ls and
// dry-run share one contract (FR-015). The only non-default format is "json".
package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// FormatJSON is the machine-readable output format selector.
const FormatJSON = "json"

// Validate rejects unsupported -o values. "" means the default (human/table) form.
func Validate(format string) error {
	switch format {
	case "", FormatJSON:
		return nil
	default:
		return fmt.Errorf("unsupported output format %q (want: json)", format)
	}
}

// JSON writes v as indented JSON to w. Slices marshal to a JSON array — pass a
// non-nil empty slice to get "[]" rather than "null" (FR-012 empty-inventory contract).
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
