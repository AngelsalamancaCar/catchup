package server

import "net/http"

// PlaceholderData es el contexto de las vistas aún no implementadas
// (matrices, organización, timeline: llegan en fases 2 y 3).
type PlaceholderData struct {
	Title   string
	Nav     string
	Message string
}

func (s *Server) handlePlaceholder(title, nav, message string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.renderFull(w, "placeholder", PlaceholderData{Title: title, Nav: nav, Message: message})
	}
}
