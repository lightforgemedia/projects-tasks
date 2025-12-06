package pt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubRunner struct {
	errFor map[string]error
	log    []string
}

func (s *stubRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	s.log = append(s.log, cmd)
	if s.errFor != nil {
		if err, ok := s.errFor[cmd]; ok {
			return []byte("failed"), err
		}
	}
	return []byte("ok"), nil
}

func TestValidateDoDCommandsPass(t *testing.T) {
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r}
	dod := DefinitionOfDone{
		Tests:         []string{"go test ./pkg"},
		ValidationCmd: "echo validate",
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil {
		t.Fatalf("ValidateDoD error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass, got fail")
	}
	if len(r.log) != 2 {
		t.Fatalf("expected two commands, got %v", r.log)
	}
}

func TestValidateDoDCommandFailure(t *testing.T) {
	r := &stubRunner{errFor: map[string]error{
		"go test ./pkg": errors.New("boom"),
	}}
	vr := ValidationRunner{Runner: r}
	dod := DefinitionOfDone{Tests: []string{"go test ./pkg"}}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err == nil {
		t.Fatalf("expected error")
	}
	if res.Passed {
		t.Fatalf("expected fail result")
	}
}

func TestValidateDoDManualRequired(t *testing.T) {
	vr := ValidationRunner{Runner: &stubRunner{}}
	dod := DefinitionOfDone{Manual: "Check UI"}
	res, err := vr.ValidateDoD(context.Background(), dod, false)
	if err == nil {
		t.Fatalf("expected manual confirmation error")
	}
	if res.Passed {
		t.Fatalf("expected failed result")
	}
}
