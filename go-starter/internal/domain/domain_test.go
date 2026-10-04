package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain", input: "ada", want: "ada"},
		{name: "trims_spaces", input: "  ada \t", want: "ada"},
		{name: "unicode", input: "Ada Lovelace", want: "Ada Lovelace"},
		{name: "empty", input: "", wantErr: true},
		{name: "only_spaces", input: "   ", wantErr: true},
		{name: "too_long", input: strings.Repeat("a", MaxNameLength+1), wantErr: true},
		{name: "max_length", input: strings.Repeat("a", MaxNameLength), want: strings.Repeat("a", MaxNameLength)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeName(%q) error = nil, want an error", tt.input)
				}
				if !errors.Is(err, ErrInvalidName) {
					t.Errorf("NormalizeName(%q) error = %v, want it to wrap ErrInvalidName", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeName(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHealthStatusOK(t *testing.T) {
	if !(HealthStatus{Status: StatusOK}).OK() {
		t.Error("status ok must report OK() = true")
	}
	if (HealthStatus{Status: StatusDegraded}).OK() {
		t.Error("status degraded must report OK() = false")
	}
}
