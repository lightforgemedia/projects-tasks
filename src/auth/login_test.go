package auth

import (
	"strings"
	"testing"
)

func TestLogin(t *testing.T) {
	token, err := Login("user", "pass")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !strings.Contains(token, "mock-token") {
		t.Errorf("expected token to contain 'mock-token', got %q", token)
	}
}

func TestLoginInvalid(t *testing.T) {
	_, err := Login("user", "wrong")
	if err == nil {
		t.Fatal("expected error, got success")
	}
}
