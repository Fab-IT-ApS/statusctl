package markdown

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	cases := []struct {
		name, in   string
		want, deny []string
	}{
		{"basic formatting", "**bold** and [link](https://example.com)\n\n- one\n- two",
			[]string{"<strong>bold</strong>", `<a href="https://example.com">link</a>`, "<li>one</li>"}, nil},
		{"mailto allowed", "[mail us](mailto:ops@example.com)", []string{`href="mailto:ops@example.com"`}, nil},
		{"raw script dropped", "<script>alert(1)</script>", nil, []string{"<script"}},
		{"inline html dropped", `hi <img src=x onerror=alert(1)>`, []string{"hi"}, []string{"<img", "onerror"}},
		{"javascript link unwrapped", "[click](javascript:alert(1))", []string{"click"}, []string{"<a", "javascript"}},
		{"javascript autolink unwrapped", "<javascript:alert(1)>", nil, []string{"<a"}},
		{"relative link unwrapped", "[x](/admin)", []string{"x"}, []string{"<a"}},
		{"data link unwrapped", "[x](data:text/html,hi)", []string{"x"}, []string{"<a"}},
		{"images become alt text", "![diagram](https://evil.example/x.png)", []string{"diagram"}, []string{"<img"}},
		{"https autolink kept", "<https://example.com>", []string{`<a href="https://example.com">`}, nil},
		{"code escaped", "`<b>`", []string{"<code>&lt;b&gt;</code>"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(Render(c.in))
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("output %q missing %q", got, w)
				}
			}
			for _, d := range c.deny {
				if strings.Contains(got, d) {
					t.Errorf("output %q must not contain %q", got, d)
				}
			}
		})
	}
}
