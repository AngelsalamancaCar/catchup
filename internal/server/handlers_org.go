package server

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"projectmapper/internal/model"
	"projectmapper/internal/scoring"
	"projectmapper/internal/svg"
)

// Umbrales de diagnóstico organizacional automático — §2.4 del plan.
const (
	coordinationRiskThreshold = 3 // sección con más de N conectores cross-sección
	thanklessReviewThreshold  = 2 // sección con al menos N Thankless Tasks
)

// sectionDiagnostics agrupa los contadores usados para las anotaciones
// automáticas de diagnóstico organizacional (swimlanes y dashboard home).
type sectionDiagnostics struct {
	crossLinks     map[string]int
	thanklessCount map[string]int
}

// computeSectionDiagnostics recorre todas las actividades una sola vez y
// cuenta, por sección: conectores cross-sección (A5, contando ambos
// extremos del enlace) y actividades en el cuadrante Thankless Tasks.
func computeSectionDiagnostics(snap model.Store, now time.Time) sectionDiagnostics {
	diag := sectionDiagnostics{crossLinks: map[string]int{}, thanklessCount: map[string]int{}}
	for _, a := range snap.Activities {
		for _, inv := range a.Involved {
			if inv == a.Section {
				continue
			}
			diag.crossLinks[a.Section]++
			diag.crossLinks[inv]++
		}
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		if sc.ImpactEffortQuadrant == scoring.QuadrantThanklessTasks {
			diag.thanklessCount[a.Section]++
		}
	}
	return diag
}

// riskSections devuelve las secciones (de la lista configurada) que superan
// el umbral de riesgo de coordinación.
func (d sectionDiagnostics) riskSections(sections []string) []string {
	var out []string
	for _, sec := range sections {
		if d.crossLinks[sec] > coordinationRiskThreshold {
			out = append(out, sec)
		}
	}
	return out
}

// badgesFor arma las anotaciones automáticas por lane (§2.4): riesgo de
// coordinación, candidata a revisión. La lane huérfana siempre recibe su
// propia anotación fija (se muestra solo si la lane termina existiendo).
func (d sectionDiagnostics) badgesFor(sections []string) map[string][]string {
	out := map[string][]string{svg.OrphanLane: {"huérfana: sin sección organizacional asignada"}}
	for _, sec := range sections {
		var badges []string
		if d.crossLinks[sec] > coordinationRiskThreshold {
			badges = append(badges, fmt.Sprintf("riesgo de coordinación (%d conectores)", d.crossLinks[sec]))
		}
		if d.thanklessCount[sec] >= thanklessReviewThreshold {
			badges = append(badges, fmt.Sprintf("candidata a revisión (%d Thankless Tasks)", d.thanklessCount[sec]))
		}
		if len(badges) > 0 {
			out[sec] = badges
		}
	}
	return out
}

// OrgSwimlanesPageData es el contexto de /org/swimlanes.
type OrgSwimlanesPageData struct {
	Title, Nav    string
	FilterSection string
	SVG           svg.SwimlaneView
}

// isOrphanSection reporta si section no está entre las configuradas (regla
// "huérfana" de §2.4).
func isOrphanSection(sections []string, section string) bool {
	return !contains(sections, section)
}

func (s *Server) handleOrgSwimlanes(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	now := time.Now().UTC()
	diag := computeSectionDiagnostics(snap, now)

	filter := r.URL.Query().Get("section")
	lanes := snap.Sections
	activities := snap.Activities
	if filter != "" {
		lanes = []string{filter}
		var filtered []model.Activity
		for _, a := range activities {
			sec := a.Section
			if isOrphanSection(snap.Sections, sec) {
				sec = svg.OrphanLane
			}
			if sec == filter {
				filtered = append(filtered, a)
			}
		}
		activities = filtered
	}

	cards := make([]svg.ActivityCardInput, 0, len(activities))
	for _, a := range activities {
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		cards = append(cards, svg.ActivityCardInput{
			ID:        a.ID,
			Name:      a.Name,
			Section:   a.Section,
			Involved:  a.Involved,
			Quadrant:  sc.ImpactEffortQuadrant,
			DetailURL: "/matrix/impact-effort/detail/" + a.ID,
		})
	}

	badges := diag.badgesFor(snap.Sections)
	view := svg.BuildSwimlanes(lanes, cards, badges, "Mapa organizacional (swimlanes)")

	s.renderFull(w, "org_swimlanes", OrgSwimlanesPageData{
		Title: "Organización — Swimlanes", Nav: "org",
		FilterSection: filter,
		SVG:           view,
	})
}

// OrgTreemapPageData es el contexto de /org/treemap.
type OrgTreemapPageData struct {
	Title, Nav string
	SVG        svg.TreemapView
}

func (s *Server) handleOrgTreemap(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()

	totals := map[string]float64{}
	for _, a := range snap.Activities {
		sec := a.Section
		if isOrphanSection(snap.Sections, sec) {
			sec = svg.OrphanLane
		}
		totals[sec] += scoring.PersonDaysMidpoint(a.Answers.C1)
	}

	laneOrder := append(append([]string{}, snap.Sections...), svg.OrphanLane)
	areas := make([]svg.SectionAreaInput, 0, len(laneOrder))
	for _, sec := range laneOrder {
		v := totals[sec]
		if v <= 0 {
			continue
		}
		areas = append(areas, svg.SectionAreaInput{
			Name:      sec,
			Value:     v,
			Color:     svg.SectionColor(snap.Sections, sec),
			DetailURL: "/org/swimlanes?section=" + url.QueryEscape(sec),
		})
	}

	s.renderFull(w, "org_treemap", OrgTreemapPageData{
		Title: "Organización — Treemap", Nav: "org",
		SVG: svg.BuildTreemap(areas, "Treemap organizacional por persona-días"),
	})
}
