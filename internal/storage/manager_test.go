package storage

import "testing"

func TestSanitizeVirtualRoot(t *testing.T) {
	valid := map[string]string{
		"Fotos":        "Fotos",
		" Documentos ": "Documentos",
		"Fotos2026":    "Fotos2026",
		"Xbox":         "Xbox",
		"/Media/":      "",
	}
	for input, want := range valid {
		if got := sanitizeVirtualRoot(input); got != want {
			t.Fatalf("sanitizeVirtualRoot(%q)=%q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"..", "a/b", `a\\b`} {
		if got := sanitizeVirtualRoot(input); got != "" {
			t.Fatalf("esperaba rechazo para %q, obtuvo %q", input, got)
		}
	}
}
