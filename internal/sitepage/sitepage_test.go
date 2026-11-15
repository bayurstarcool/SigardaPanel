package sitepage

import (
	"strings"
	"testing"
)

func TestHTML(t *testing.T) {
	for _, rt := range []string{"static", "php", "node", "python", "go", "docker", "unknown"} {
		h := HTML("shop.example.com", rt)
		if !strings.HasPrefix(h, "<!DOCTYPE html>") || !strings.Contains(h, "<h1") || !strings.Contains(h, "shop.example.com") {
			t.Fatalf("%s: bad page", rt)
		}
		if strings.Contains(h, "/*TAILWIND*/") || !strings.Contains(h, "--color-slate-950") {
			t.Fatalf("%s: CSS not inlined", rt)
		}
		if strings.Contains(h, "http://") && !strings.Contains(h, "http://www.w3.org/2000/svg") {
			t.Fatalf("%s: external reference", rt)
		}
	}
	if h := HTML(`<script>x</script>`, "static"); strings.Contains(h, "<script>x") {
		t.Fatal("domain not escaped")
	}
}
