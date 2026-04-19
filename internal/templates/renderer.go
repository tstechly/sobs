package templates

import (
	"path/filepath"

	"github.com/flosch/pongo2/v6"
)

type Renderer struct {
	set *pongo2.TemplateSet
}

func NewRenderer(templateRoot string) (*Renderer, error) {
	loader, err := pongo2.NewLocalFileSystemLoader(templateRoot)
	if err != nil {
		return nil, err
	}
	return &Renderer{set: pongo2.NewSet("sobs", loader)}, nil
}

func (r *Renderer) Render(name string, context pongo2.Context) (string, error) {
	tpl, err := r.set.FromFile(filepath.Clean(name))
	if err != nil {
		return "", err
	}
	return tpl.Execute(context)
}
