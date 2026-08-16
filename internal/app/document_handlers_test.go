package app

import "testing"

func TestFileViewerKindAndEditingPolicy(t *testing.T) {
	tests := []struct {
		name     string
		viewer   string
		editable bool
	}{
		{"README.md", "markdown", true},
		{"manual.markdown", "markdown", true},
		{"index.HTML", "html", true},
		{"notas.txt", "text", true},
		{"registro.log", "text", true},
		{"guia.pdf", "pdf", false},
		{"foto.jpg", "image", false},
		{"clip.mp4", "video", false},
		{"cancion.flac", "audio", false},
		{"datos.json", "text", true},
		{"main.go", "text", true},
		{"app.apk", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileViewerKind(tt.name); got != tt.viewer {
				t.Fatalf("viewer=%q want=%q", got, tt.viewer)
			}
			if got := fileViewerEditable(tt.name); got != tt.editable {
				t.Fatalf("editable=%v want=%v", got, tt.editable)
			}
		})
	}
}

func TestTextVersionChangesWithContentMetadata(t *testing.T) {
	base := textVersion(1234, 20)
	if base == textVersion(1235, 20) {
		t.Fatal("cambiar la fecha debe cambiar la versión optimista")
	}
	if base == textVersion(1234, 21) {
		t.Fatal("cambiar el tamaño debe cambiar la versión optimista")
	}
}
