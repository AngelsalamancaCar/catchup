package server

import "net/http"

// HelpPageData es el contexto de `GET /help`.
type HelpPageData struct {
	Title, Nav string
}

// handleHelp sirve la guía de usuario de una página (Fase 5, tarea 5): anclas
// de cada escala y significado de cada cuadrante, sin estado ni parámetros.
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	s.renderFull(w, "help", HelpPageData{Title: "Ayuda", Nav: "help"})
}
