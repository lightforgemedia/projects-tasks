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
	want := []string{"sh -c go test ./pkg", "sh -c echo validate"}
	if len(r.log) != len(want) {
		t.Fatalf("expected %d commands, got %v", len(want), r.log)
	}
	for i, cmd := range want {
		if r.log[i] != cmd {
			t.Fatalf("command %d mismatch: want %q got %q", i, cmd, r.log[i])
		}
	}
}

func TestValidateDoDCommandFailure(t *testing.T) {
	r := &stubRunner{errFor: map[string]error{
		"sh -c go test ./pkg": errors.New("boom"),
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
	if !strings.Contains(res.Output, "go test ./pkg") {
		t.Errorf("output should contain failing command, got: %q", res.Output)
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

func TestValidateDoDQuotedCommand(t *testing.T) {
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r}
	dod := DefinitionOfDone{
		Tests: []string{`bash -c "echo 'hello world'"`},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil || !res.Passed {
		t.Fatalf("expected pass, got err=%v res=%+v", err, res)
	}
	if got := r.log; len(got) != 1 || got[0] != `sh -c bash -c "echo 'hello world'"` {
		t.Fatalf("unexpected command log: %v", got)
	}
}
