package main

import "testing"

func TestCmdSyncErrors(t *testing.T) {
	// 1. Missing Argument
	if err := cmdSync([]string{}); err == nil {
		t.Error("expected error for missing manifest argument")
	}

	// 2. Invalid Manifest Path
	if err := cmdSync([]string{"nonexistent.json"}); err == nil {
		t.Error("expected error for invalid manifest path")
	}
}

func TestCmdClaimErrors(t *testing.T) {
	if err := cmdClaim([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
}

func TestCmdReleaseErrors(t *testing.T) {
	if err := cmdRelease([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
}

func TestCmdRejectErrors(t *testing.T) {
	if err := cmdReject([]string{}); err == nil {
		t.Error("expected error for missing id")
	}

	if err := cmdReject([]string{"ID"}); err == nil {
		t.Error("expected error for missing reason")
	}

}

func TestCmdValidateErrors(t *testing.T) {
	if err := cmdValidate([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
}

func TestCmdApproveErrors(t *testing.T) {
	if err := cmdApprove([]string{}); err == nil {
		t.Error("expected error for missing id")
	}
}
