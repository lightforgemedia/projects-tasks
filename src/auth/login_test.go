package auth

import (
	"strings"
	"sync/atomic"
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

func TestLoginMetrics(t *testing.T) {
	atomic.StoreUint64(&loginSuccessAttempts, 0)
	atomic.StoreUint64(&loginFailureAttempts, 0)

	_, _ = Login("user", "pass")
	_, _ = Login("user", "wrong")

	if got := atomic.LoadUint64(&loginSuccessAttempts); got != 1 {
		t.Fatalf("success attempts=%d, want 1", got)
	}
	if got := atomic.LoadUint64(&loginFailureAttempts); got != 1 {
		t.Fatalf("failure attempts=%d, want 1", got)
	}
}
