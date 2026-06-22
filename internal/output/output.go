package output

import (
	"encoding/json"
	"fmt"
	"io"
)

const FormatJSON = "json"

func Validate(format string) error {
	switch format {
	case "", FormatJSON:
		return nil
	default:
		return fmt.Errorf("unsupported output format %q (want: json)", format)
	}
}

func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
