package mailbody

import (
	"strings"
	"testing"
)

func TestUntrustedHTML(t *testing.T) {
	source := `<head><style>body{display:none}</style></head><p onclick="steal()">Hello <b>world</b><img src="cid:safe" onerror="steal()"><img src="javascript:bad()"><a href="javascript:bad()">bad</a><a href="https://example.test">good</a></p><svg><script>steal()</script></svg><iframe src="/api/state"></iframe><form action="/api/settings"><input></form><span style="background:url(https://tracking.test)">text</span>`
	clean := Clean(source)
	for _, bad := range []string{"script", "onclick", "onerror", "steal()", "<svg", "<iframe", "<form", "background", "tracking.test"} {
		if strings.Contains(clean, bad) {
			t.Fatalf("active content %q: %s", bad, clean)
		}
	}
	for _, good := range []string{"<b>world</b>", `src="cid:safe"`, `href="https://example.test"`} {
		if !strings.Contains(clean, good) {
			t.Fatalf("lost %q", good)
		}
	}
}
