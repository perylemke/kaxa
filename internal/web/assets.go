package web

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed assets/index.html assets/app.css assets/app.js
var assetFS embed.FS

var indexPage = template.Must(template.ParseFS(assetFS, "assets/index.html"))

type indexData struct {
	Session string
	CSRF    string
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", contentHTML)
	_ = indexPage.ExecuteTemplate(w, "index.html", indexData{
		Session: s.sessionHex,
		CSRF:    s.csrfHex,
	})
}

func (s *Server) serveCSS(w http.ResponseWriter, r *http.Request) {
	s.serveAsset(w, r, "assets/app.css", contentCSS)
}

func (s *Server) serveJS(w http.ResponseWriter, r *http.Request) {
	s.serveAsset(w, r, "assets/app.js", contentJS)
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request, name, contentType string) {
	data, err := assetFS.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
