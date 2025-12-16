package demo

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDemoServerRoutesContainStableElements(t *testing.T) {
	s, err := Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})

	type check struct {
		path string
		want []string
	}
	for _, tc := range []check{
		{path: "/products?query=socks", want: []string{`aria-label="Quick Add"`, `data-testid="mini-cart"`}},
		{path: "/login", want: []string{`for="email"`, `for="password"`, `aria-label="Sign in"`}},
		{path: "/settings/profile", want: []string{`aria-label="Edit profile"`, `aria-label="Save"`, `data-testid="toast"`, `>Saved<`}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(s.BaseURL + tc.path)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			body := string(raw)
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Fatalf("expected %q in response body", w)
				}
			}
		})
	}
}
