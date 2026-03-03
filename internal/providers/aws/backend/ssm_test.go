package aws

import (
	"testing"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"1", 1, false},
		{"443", 443, false},
		{"8080", 8080, false},
		{"65535", 65535, false},
		{"0", 0, true},
		{"65536", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
		{"3.14", 0, true},
	}

	for _, tt := range tests {
		got, err := ValidatePort(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("ValidatePort(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}