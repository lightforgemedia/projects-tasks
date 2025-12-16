//go:build integration

package pulse

import (
	"context"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func TestRodCanLaunchNavigateAndEval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("about:blank")
	page.MustWaitLoad()

	val := page.MustEval("() => ({ ok: true, title: document.title })")
	if val.Value.Object() == nil {
		t.Fatalf("expected an object result")
	}
}

