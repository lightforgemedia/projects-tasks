package pt

import "testing"

func TestParseStringArrayCommaInQuotes(t *testing.T) {
	got, err := parseStringArray(`["echo 'a,b'"]`)
	if err != nil {
		t.Fatalf("parseStringArray: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1: %#v", len(got), got)
	}
	if got[0] != "echo 'a,b'" {
		t.Fatalf("got %q, want %q", got[0], "echo 'a,b'")
	}
}

