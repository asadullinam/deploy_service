package http

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/*
var uiAssets embed.FS

func (h *Handler) HomeUI(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(uiAssets, "ui/index.html")
	if err != nil {
		http.Error(w, "ui is unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}

func (h *Handler) UIAssets() http.Handler {
	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
