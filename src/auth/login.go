package auth

import (
	"errors"
	"sync/atomic"
)

var loginSuccessAttempts uint64
var loginFailureAttempts uint64

// Login performs a mock login and returns a token.
func Login(username, password string) (string, error) {
	if username == "user" && password == "pass" {
		atomic.AddUint64(&loginSuccessAttempts, 1)
		return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.mock-token", nil
	}
	atomic.AddUint64(&loginFailureAttempts, 1)
	return "", errors.New("invalid credentials")
}
