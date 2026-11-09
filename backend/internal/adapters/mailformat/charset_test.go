package mailformat

import (
	"strings"
	"testing"
)

func TestRussianCharsetAndSafeHTML(t *testing.T) {
	raw := "From: =?windows-1251?B?z+Xy8A==?= <p@example.test>\r\nSubject: =?windows-1251?B?z/Do4uXy?=\r\nContent-Type: text/plain; charset=windows-1251\r\nContent-Transfer-Encoding: base64\r\n\r\nz/Do4uXy"
	m, e := Decode([]byte(raw))
	if e != nil || m.Subject != "Привет" || m.Body != "Привет" || m.FromName != "Петр" {
		t.Fatalf("charset decode: %+v %v", m, e)
	}
	raw = "Subject: HTML\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html><head><style>hidden</style></head><body><p>Привет &amp; hello</p><script>bad()</script><p>Next<img src='https://example.test/tracker'></p></body></html>"
	m, e = Decode([]byte(raw))
	if e != nil || m.Body != "Привет & hello\nNext" {
		t.Fatalf("safe html: %q %v", m.Body, e)
	}
	if strings.Contains(m.Body, "tracker") {
		t.Fatal("html leaked")
	}
}
