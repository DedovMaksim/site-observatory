package web

import (
	"html/template"
 "net/http"
 "net/http/httptest"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	_, err := template.New("base").Funcs(pageTemplateFuncs()).ParseFS(assets, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
}

func TestRoutesDoNotConflictWithAPI(t *testing.T) {
 s:=&Server{mux:http.NewServeMux()}
 // Regression: a methodless /api/v1/ overlaps GET / and panics
 // when registered with the Go 1.22+ ServeMux.
 s.routes()
 cases:=[]struct{method,path,want string}{
  {"GET","/","GET /"},
  {"GET","/api/v1/summary","GET /api/v1/"},
  {"GET","/api/v1/monitors","GET /api/v1/"},
  {"POST","/api/v1/summary",""},
 }
 for _,tc:=range cases{
  req:=httptest.NewRequest(tc.method,tc.path,nil)
  _,pattern:=s.mux.Handler(req)
  if pattern!=tc.want{t.Errorf("%s %s: matched %q, want %q",tc.method,tc.path,pattern,tc.want)}
 }
}
