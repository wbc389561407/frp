package appstate

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "shell.json")

	s, err := Load(path)
	if err != nil || s != nil {
		t.Fatalf("missing file should return nil state, got %v, %v", s, err)
	}

	want := Default("pw")
	want.FrpcVersion = "0.61.1"
	want.Onboarded = true
	if err := want.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if *got != *want {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, want)
	}
}
