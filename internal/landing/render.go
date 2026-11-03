package landing

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"time"
)

type Renderer struct {
	pages map[string]*template.Template
	frags map[string]*template.Template
}

func newRenderer(fsys fs.FS) (*Renderer, error) {
	templatesFS, err := fs.Sub(fsys, "templates")
	if err != nil {
		return nil, fmt.Errorf("templates fs: %w", err)
	}

	r := &Renderer{
		pages: make(map[string]*template.Template),
		frags: make(map[string]*template.Template),
	}

	if err := r.parsePages(templatesFS); err != nil {
		return nil, err
	}
	if err := r.parseFragments(templatesFS); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Renderer) parsePages(templatesFS fs.FS) error {
	pages, err := fs.Glob(templatesFS, "pages/*.html")
	if err != nil {
		return fmt.Errorf("list pages: %w", err)
	}
	for _, page := range pages {
		name := strings.TrimSuffix(strings.TrimPrefix(page, "pages/"), ".html")
		t, err := template.New("base").Funcs(funcMap()).ParseFS(templatesFS,
			"base.html", "partials/*.html", "fragments/*.html", page)
		if err != nil {
			return fmt.Errorf("parse page %s: %w", page, err)
		}
		r.pages[name] = t
	}
	return nil
}

func (r *Renderer) parseFragments(templatesFS fs.FS) error {
	fragments, err := fs.Glob(templatesFS, "fragments/*.html")
	if err != nil {
		return fmt.Errorf("list fragments: %w", err)
	}
	for _, fragment := range fragments {
		name := strings.TrimSuffix(strings.TrimPrefix(fragment, "fragments/"), ".html")
		t, err := template.New(name).Funcs(funcMap()).ParseFS(templatesFS,
			"partials/*.html", fragment)
		if err != nil {
			return fmt.Errorf("parse fragment %s: %w", fragment, err)
		}
		r.frags[name] = t
	}
	return nil
}

func (r *Renderer) Render(w io.Writer, page string, data PageData) error {
	t, ok := r.pages[page]
	if !ok {
		return fmt.Errorf("unknown page template %q", page)
	}
	return t.ExecuteTemplate(w, "base", data)
}

func (r *Renderer) RenderFragment(w io.Writer, name string, data PageData) error {
	t, ok := r.frags[name]
	if !ok {
		return fmt.Errorf("unknown fragment %q", name)
	}
	return t.ExecuteTemplate(w, name, data)
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"icon": func(name string) template.HTML {
			if name == "github" {
				return template.HTML(`<svg class="icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">` + githubPath + `</svg>`)
			}
			path, ok := iconPaths[name]
			if !ok {
				return ""
			}
			return template.HTML(`<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` + path + `</svg>`)
		},
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"year":     func() int { return time.Now().Year() },
		"tel":      telHref,
	}
}

func telHref(phone string) string {
	replacer := strings.NewReplacer(" ", "", "(", "", ")", "", "-", "", ".", "")
	return "tel:" + replacer.Replace(phone)
}
