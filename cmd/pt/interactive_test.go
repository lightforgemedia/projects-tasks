package main

import (
	"os"
	"testing"
)

func TestConfirmManual_AutoYes(t *testing.T) {
	if !confirmManual([]string{"step 1"}, true) {
		t.Error("expected true when autoYes is true")
	}
}

func TestConfirmManual_InteractiveYes(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	os.Stdin = r

	go func() {
		w.Write([]byte("y\n"))
		w.Close()
	}()

	// We can't easily suppress stdout of the function under test without refactoring,
	// but it won't break the test, just show up in logs.
	if !confirmManual([]string{"step 1"}, false) {
		t.Error("expected true when user inputs 'y'")
	}
}

func TestConfirmManual_InteractiveNo(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	os.Stdin = r

	go func() {
		w.Write([]byte("n\n"))
		w.Close()
	}()

	if confirmManual([]string{"step 1"}, false) {
		t.Error("expected false when user inputs 'n'")
	}
}
