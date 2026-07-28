package server

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"projectmapper/internal/model"
)

const weightSumEpsilon = 0.005

// ConfigPageData es el contexto de la página de configuración.
type ConfigPageData struct {
	Title       string
	Nav         string
	Sections    []string
	SectionsErr string
	Weights     model.Weights
	WeightsErr  string
}

func (s *Server) configPageData() ConfigPageData {
	return ConfigPageData{
		Title:    "Configuración",
		Nav:      "config",
		Sections: s.store.Sections(),
		Weights:  s.store.Weights(),
	}
}

func (s *Server) handleConfigPage(w http.ResponseWriter, r *http.Request) {
	s.renderFull(w, "config", s.configPageData())
}

func (s *Server) handleConfigSections(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	raw := r.FormValue("sections")
	var sections []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			sections = append(sections, line)
		}
	}

	data := s.configPageData()
	if len(sections) == 0 {
		data.SectionsErr = "Debes configurar al menos una sección."
		data.Sections = nil
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderFull(w, "config", data)
		return
	}

	if err := s.store.SetSections(sections); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/config", http.StatusSeeOther)
}

func parseWeight(r *http.Request, field string) float64 {
	v, err := strconv.ParseFloat(r.FormValue(field), 64)
	if err != nil {
		return -1
	}
	return v
}

func (s *Server) handleConfigWeights(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}

	impact := map[string]float64{
		"B1": parseWeight(r, "b1"),
		"B2": parseWeight(r, "b2"),
		"B3": parseWeight(r, "b3"),
		"B4": parseWeight(r, "b4"),
	}
	effort := map[string]float64{
		"C1": parseWeight(r, "c1"),
		"C2": parseWeight(r, "c2"),
		"C3": parseWeight(r, "c3"),
		"C4": parseWeight(r, "c4"),
	}

	sumImpact, sumEffort := 0.0, 0.0
	valid := true
	for _, v := range impact {
		if v < 0 {
			valid = false
		}
		sumImpact += v
	}
	for _, v := range effort {
		if v < 0 {
			valid = false
		}
		sumEffort += v
	}

	data := s.configPageData()
	data.Weights = model.Weights{Impact: impact, Effort: effort}

	if !valid {
		data.WeightsErr = "Todos los pesos deben ser números válidos entre 0 y 1."
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderFull(w, "config", data)
		return
	}
	if math.Abs(sumImpact-1.0) > weightSumEpsilon || math.Abs(sumEffort-1.0) > weightSumEpsilon {
		data.WeightsErr = fmt.Sprintf(
			"Los pesos deben sumar 1.0 por dimensión (Impacto = %.2f, Esfuerzo = %.2f).",
			sumImpact, sumEffort)
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderFull(w, "config", data)
		return
	}

	if err := s.store.SetWeights(model.Weights{Impact: impact, Effort: effort}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/config", http.StatusSeeOther)
}
