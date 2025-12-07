package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Default cmd tests to bd backend so scripted runner works.
	os.Setenv("PT_BACKEND", "bd")
	os.Exit(m.Run())
}
