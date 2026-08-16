package storage

import "testing"

func TestFileKindAndIconRecognizeCommonSchoolFiles(t *testing.T) {
	tests := []struct {
		name      string
		wantKind  string
		wantIcon  string
		wantLabel string
	}{
		{"app-release.apk", "other", "android", "APK"},
		{"tarea.pdf", "document", "pdf", "PDF"},
		{"README.md", "document", "markdown", "MD"},
		{"manual.mdown", "document", "markdown", "MD"},
		{"notas.text", "document", "text", "TEXT"},
		{"index.xhtml", "document", "code", "XHTML"},
		{"ensayo.docx", "document", "word", "DOC"},
		{"calificaciones.xlsx", "document", "excel", "XLS"},
		{"exposicion.pptx", "document", "powerpoint", "PPT"},
		{"datos.sqlite3", "other", "database", "DB"},
		{"respaldo.7z", "archive", "archive", "7Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := FileKind(tt.name)
			if kind != tt.wantKind {
				t.Fatalf("FileKind(%q)=%q want %q", tt.name, kind, tt.wantKind)
			}
			icon, label := FileIcon(tt.name, kind)
			if icon != tt.wantIcon || label != tt.wantLabel {
				t.Fatalf("FileIcon(%q)=%q/%q want %q/%q", tt.name, icon, label, tt.wantIcon, tt.wantLabel)
			}
		})
	}
}
