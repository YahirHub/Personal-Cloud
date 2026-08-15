package webui

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"

	webassets "personalcloud/web"
)

type Renderer struct {
	base *template.Template
}

func NewRenderer() (*Renderer, error) {
	base, err := template.New("base").ParseFS(webassets.Assets, "layouts/*.html", "components/*.html")
	if err != nil {
		return nil, fmt.Errorf("cargar layout: %w", err)
	}
	return &Renderer{base: base}, nil
}

func (r *Renderer) Render(w io.Writer, page string, data any) error {
	clone, err := r.base.Clone()
	if err != nil {
		return err
	}
	if _, err := clone.ParseFS(webassets.Assets, "pages/"+page+".html"); err != nil {
		return fmt.Errorf("cargar página %q: %w", page, err)
	}
	return clone.ExecuteTemplate(w, "base", data)
}

func StaticFS() (fs.FS, error) {
	return fs.Sub(webassets.Assets, "static")
}
