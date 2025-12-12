package auth

import "errors"

// Login performs a mock login and returns a token.
func Login(username, password string) (string, error) {
	if username == "user" && password == "pass" {
		return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.mock-token", nil
	}
	return "", errors.New("invalid credentials")
}
