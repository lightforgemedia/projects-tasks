//go:build integration

package pulse

import (
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func TestRodCanLaunchNavigateAndEval(t *testing.T) {
	u := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("about:blank").Timeout(30 * time.Second)
	page.MustWaitLoad()

	val := page.MustEval("() => ({ ok: true, title: document.title })")
	if !val.Get("ok").Bool() {
		t.Fatalf("expected ok=true, got: %s", val.String())
	}
}
