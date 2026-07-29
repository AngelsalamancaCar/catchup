package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"projectmapper/internal/scoring"
)

// DashboardCard es una tarjeta resumen del home, con link a la vista detalle.
type DashboardCard struct {
	Title string
	Lines []string
	Link  string
}

// HomePageData es el contexto de `GET /`.
type HomePageData struct {
	Title, Nav string
	Cards      []DashboardCard
}

// handleHome arma las 4 tarjetas resumen del dashboard (§3 Fase 3, tarea 5):
// actividades por cuadrante Eisenhower, entregables at-risk, actividades que
// necesitan información y secciones con riesgo de coordinación.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	now := time.Now().UTC()

	eisenhowerCounts := map[string]int{}
	needsInfo := 0
	for _, a := range snap.Activities {
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		eisenhowerCounts[sc.EisenhowerQuadrant]++
		if sc.NeedsInfo {
			needsInfo++
		}
	}

	nodes := buildDeliverableNodes(snap.Activities, now)
	atRisk := resolveAtRisk(nodes)
	atRiskCount := 0
	for _, v := range atRisk {
		if v {
			atRiskCount++
		}
	}

	diag := computeSectionDiagnostics(snap, now)
	riskSections := diag.riskSections(snap.Sections)
	riskLine := fmt.Sprintf("%d sección(es)", len(riskSections))
	if len(riskSections) > 0 {
		riskLine += ": " + strings.Join(riskSections, ", ")
	}

	cards := []DashboardCard{
		{
			Title: "Actividades por cuadrante Eisenhower",
			Lines: []string{
				fmt.Sprintf("Hacer: %d", eisenhowerCounts[scoring.EisenhowerDo]),
				fmt.Sprintf("Agendar: %d", eisenhowerCounts[scoring.EisenhowerSchedule]),
				fmt.Sprintf("Delegar: %d", eisenhowerCounts[scoring.EisenhowerDelegate]),
				fmt.Sprintf("Eliminar: %d", eisenhowerCounts[scoring.EisenhowerDelete]),
			},
			Link: "/matrix/eisenhower",
		},
		{
			Title: "Entregables at-risk",
			Lines: []string{fmt.Sprintf("%d de %d entregables", atRiskCount, len(nodes))},
			Link:  "/timeline",
		},
		{
			Title: "Actividades que necesitan información",
			Lines: []string{fmt.Sprintf("%d de %d actividades", needsInfo, len(snap.Activities))},
			Link:  "/matrix/eisenhower",
		},
		{
			Title: "Secciones con riesgo de coordinación",
			Lines: []string{riskLine},
			Link:  "/org/swimlanes",
		},
	}

	s.renderFull(w, "home", HomePageData{Title: "Inicio", Nav: "home", Cards: cards})
}
