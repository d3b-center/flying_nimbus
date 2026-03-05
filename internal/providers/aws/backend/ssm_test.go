package aws

import (
	"testing"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"1", false},
		{"443", false},
		{"8080", false},
		{"65535", false},
		{"0", true},
		{"65536", true},
		{"-1", true},
		{"abc", true},
		{"", true},
		{"3.14", true},
	}

	for _, tt := range tests {
		err := ValidatePort(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
	}
}
