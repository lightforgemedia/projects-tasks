package main

import "testing"

func TestMessage(t *testing.T) {
	want := "example-project: hello from pt demo"
	if got := Message(); got != want {
		t.Fatalf("Message() = %q, want %q", got, want)
	}
}
