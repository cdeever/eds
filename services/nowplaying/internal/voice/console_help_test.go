package voice

import (
	"html"
	"os"
	"regexp"
	"testing"
)

// The console's "How to ask" lists what voiced understands (ADR-0055). Each
// example on it is read here as voiced reads it, so that a change to the
// grammar that leaves the page wrong fails, and the page cannot promise a
// request voiced would not understand.
func TestTheConsolesHowToAskIsUnderstood(t *testing.T) {
	page, err := os.ReadFile("../../../console/internal/web/static/index.html")
	if err != nil {
		t.Skipf("the console's page is not beside this module: %v", err)
	}
	says := regexp.MustCompile(`<q class="say">([^<]+)</q>`).FindAllStringSubmatch(string(page), -1)
	if len(says) < 10 {
		t.Fatalf("only %d examples found on the page", len(says))
	}
	for _, m := range says {
		words := html.UnescapeString(m[1])
		if _, err := Parse(words); err == nil {
			continue
		}
		// A name alone is read with the library, which voiced has.
		if _, err := ParseName(words); err == nil {
			continue
		}
		t.Errorf("the console says %q, which voiced does not understand", words)
	}
}
