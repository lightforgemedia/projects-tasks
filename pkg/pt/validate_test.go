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

func TestValidateDoDStripsOuterQuotes(t *testing.T) {
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r}
	dod := DefinitionOfDone{
		Tests: []string{`"echo ok"`},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil || !res.Passed {
		t.Fatalf("expected pass, got err=%v res=%+v", err, res)
	}
	if got := r.log; len(got) != 1 || got[0] != `sh -c echo ok` {
		t.Fatalf("unexpected command log: %v", got)
	}
}

func TestValidateDoDSkipsDashNoop(t *testing.T) {
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r}
	dod := DefinitionOfDone{
		Tests: []string{"-", "echo ok"},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil || !res.Passed {
		t.Fatalf("expected pass, got err=%v res=%+v", err, res)
	}
	if got := r.log; len(got) != 1 || got[0] != `sh -c echo ok` {
		t.Fatalf("unexpected command log: %v", got)
	}
}

func TestValidateDoDRewritesPTSelf(t *testing.T) {
	t.Setenv("PT_SELF", "/abs/path/to/pt")
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r}
	dod := DefinitionOfDone{
		Tests: []string{"pt review check pt-7 --kind=pre"},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil || !res.Passed {
		t.Fatalf("expected pass, got err=%v res=%+v", err, res)
	}
	if got := r.log; len(got) != 1 || got[0] != `sh -c /abs/path/to/pt review check pt-7 --kind=pre` {
		t.Fatalf("unexpected command log: %v", got)
	}
}

func TestValidateDoDExpandsThisTaskIDPlaceholder(t *testing.T) {
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r, TaskID: "pt-999"}
	dod := DefinitionOfDone{
		Tests: []string{"echo <this-task-id>"},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil || !res.Passed {
		t.Fatalf("expected pass, got err=%v res=%+v", err, res)
	}
	if got := r.log; len(got) != 1 || got[0] != "sh -c echo pt-999" {
		t.Fatalf("unexpected command log: %v", got)
	}
}

func TestValidateDoDExpandsPlaceholderBeforePTSelfRewrite(t *testing.T) {
	t.Setenv("PT_SELF", "/abs/path/to/pt")
	r := &stubRunner{}
	vr := ValidationRunner{Runner: r, TaskID: "pt-123"}
	dod := DefinitionOfDone{
		Tests: []string{"pt review check <this-task-id> --kind=post"},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil || !res.Passed {
		t.Fatalf("expected pass, got err=%v res=%+v", err, res)
	}
	if got := r.log; len(got) != 1 || got[0] != "sh -c /abs/path/to/pt review check pt-123 --kind=post" {
		t.Fatalf("unexpected command log: %v", got)
	}
}

func TestValidateDoDWorkDirWired(t *testing.T) {
	// When Runner is nil, WorkDir should be passed to the default ExecRunner
	tmpDir := t.TempDir()

	// Create a test script that prints pwd
	vr := ValidationRunner{WorkDir: tmpDir}
	dod := DefinitionOfDone{
		Tests: []string{"pwd"},
	}
	res, err := vr.ValidateDoD(context.Background(), dod, true)
	if err != nil {
		t.Fatalf("ValidateDoD error: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected pass")
	}
	// The output should contain the temp directory path
	if !strings.Contains(res.Output, tmpDir) {
		t.Errorf("expected output to contain workdir %q, got: %q", tmpDir, res.Output)
	}
}
