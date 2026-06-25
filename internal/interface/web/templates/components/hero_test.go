package components

import (
	"context"
	"strings"
	"testing"
)

// TestHeroHeaderDelegateLink pins the delegate-dashboard link: it must appear in
// the header only when the delegate feature is enabled. Without the feature
// enabled the link must be absent (and historically the /delegate page was only
// reachable by guessing the URL, which this link fixes).
func TestHeroHeaderDelegateLink(t *testing.T) {
	render := func(delegateEnabled bool) string {
		var b strings.Builder
		if err := HeroHeader(delegateEnabled).Render(context.Background(), &b); err != nil {
			t.Fatalf("render: %v", err)
		}
		return b.String()
	}

	if got := render(true); !strings.Contains(got, `href="/delegate"`) {
		t.Errorf("delegate enabled: expected a /delegate link, got:\n%s", got)
	}
	if got := render(false); strings.Contains(got, `href="/delegate"`) {
		t.Errorf("delegate disabled: expected no /delegate link, got:\n%s", got)
	}
}
