package web

import (
	"html/template"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	_, err := template.New("base").Funcs(pageTemplateFuncs()).ParseFS(assets, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
}
