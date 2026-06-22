package output

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		format  string
		wantErr bool
	}{
		{"", false},
		{"json", false},
		{"yaml", true},
		{"JSON", true}, // case-sensitive: only lowercase "json" is the contract
		{"table", true},
	} {
		err := Validate(tc.format)
		if (err != nil) != tc.wantErr {
			t.Errorf("Validate(%q) error = %v, wantErr = %v", tc.format, err, tc.wantErr)
		}
	}
}

func TestValidateRejectionNamesTheFormat(t *testing.T) {
	err := Validate("xml")
	if err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Errorf("expected the error to name the offending format, got: %v", err)
	}
}

func TestJSONEmptySliceMarshalsToArrayNotNull(t *testing.T) {
	// The FR-012 empty-inventory contract: `ls -o json` with no sessions emits "[]".
	// A nil slice would marshal to "null"; a non-nil empty slice must give "[]".
	var b strings.Builder
	if err := JSON(&b, []string{}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got := strings.TrimSpace(b.String()); got != "[]" {
		t.Errorf("expected %q for an empty slice, got %q", "[]", got)
	}
}

func TestJSONNilSliceMarshalsToNull(t *testing.T) {
	// Documents the trap the empty-inventory contract guards against: callers MUST
	// pass a non-nil slice. A nil slice marshals to "null", which fails the contract.
	var b strings.Builder
	var nilSlice []string
	if err := JSON(&b, nilSlice); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got := strings.TrimSpace(b.String()); got != "null" {
		t.Errorf("expected %q for a nil slice, got %q", "null", got)
	}
}

func TestJSONEncodesFields(t *testing.T) {
	var b strings.Builder
	if err := JSON(&b, struct {
		SessionID string `json:"sessionID"`
	}{SessionID: "abc123"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, `"sessionID": "abc123"`) {
		t.Errorf("expected indented JSON with the field, got: %q", out)
	}
}
