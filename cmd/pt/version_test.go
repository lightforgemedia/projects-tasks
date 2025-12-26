package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVersionFlagAndCommand(t *testing.T) {
	for _, args := range [][]string{
		{"pt", "--version"},
		{"pt", "version"},
		{"pt", "version", "--json"},
	} {
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := run(args)

			w.Close()
			os.Stdout = old

			if err != nil {
				t.Fatalf("run %v: %v", args, err)
			}
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			out := strings.TrimSpace(buf.String())
			if out == "" {
				t.Fatalf("expected non-empty output for %v", args)
			}
		})
	}
}
