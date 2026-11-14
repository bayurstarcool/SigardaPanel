package nginx

import (
	"strings"
	"testing"
)

func TestStripIPv6Listen(t *testing.T) {
	in := "server {\n    listen 80;\n    listen [::]:80;\n    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n    server_name a.test;\n}\n"
	out := StripIPv6Listen(in)
	if strings.Contains(out, "[::]") {
		t.Fatalf("IPv6 listen kept:\n%s", out)
	}
	for _, keep := range []string{"listen 80;", "listen 443 ssl http2;", "server_name a.test;"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("%q lost:\n%s", keep, out)
		}
	}
}
