package server

import (
	"net/http"
	"sort"
	"time"

	"projectmapper/internal/model"
	"projectmapper/internal/scoring"
	"projectmapper/internal/svg"
)

// atRiskWindowDays es la ventana de "vence pronto" de la regla at-risk — §2.4.
const atRiskWindowDays = 14

// deliverableNode es un entregable resuelto con su clave global única. Los
// IDs de entregable (E1's ID) se reinician por actividad, así que la clave
// real es ActivityID + "#" + Deliverable.ID.
type deliverableNode struct {
	key          string
	activityID   string
	activityName string
	name, status string
	due          time.Time
	baseAtRisk   bool
	dependsOnKey string
}

func deliverableKey(activityID, deliverableID string) string {
	return activityID + "#" + deliverableID
}

// buildDeliverableNodes resuelve las fechas y arma el grafo de dependencias
// (E4) de todos los entregables. Un entregable con fecha inválida (dato
// corrupto fuera del flujo normal del cuestionario, que siempre valida la
// fecha) se descarta en este boundary en vez de propagarse al resto.
func buildDeliverableNodes(activities []model.Activity, now time.Time) []deliverableNode {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, atRiskWindowDays)

	var nodes []deliverableNode
	for _, a := range activities {
		for _, d := range a.Deliverables {
			due, err := time.Parse("2006-01-02", d.Due)
			if err != nil {
				continue
			}
			dependsOnKey := ""
			if d.DependsOn != nil && *d.DependsOn != "" {
				dependsOnKey = deliverableKey(a.ID, *d.DependsOn)
			}
			nodes = append(nodes, deliverableNode{
				key: deliverableKey(a.ID, d.ID), activityID: a.ID, activityName: a.Name,
				name: d.Name, status: d.Status, due: due,
				baseAtRisk:   d.Status != "done" && due.Before(cutoff),
				dependsOnKey: dependsOnKey,
			})
		}
	}
	return nodes
}

// resolveAtRisk propaga el estado at-risk transitivamente por la cadena de
// dependencias (E4): un entregable también es at-risk si aquel del que
// depende lo es. Usa un set "en curso" para detectar ciclos (A→B→A) y
// romperlos sin recursión descontrolada — nunca cuelga.
func resolveAtRisk(nodes []deliverableNode) map[string]bool {
	byKey := make(map[string]deliverableNode, len(nodes))
	for _, n := range nodes {
		byKey[n.key] = n
	}
	resolved := map[string]bool{}
	visiting := map[string]bool{}

	var resolve func(key string) bool
	resolve = func(key string) bool {
		if v, ok := resolved[key]; ok {
			return v
		}
		if visiting[key] {
			return false // ciclo detectado: no propagar más por esta rama
		}
		n, ok := byKey[key]
		if !ok {
			return false
		}
		visiting[key] = true
		result := n.baseAtRisk
		if !result && n.dependsOnKey != "" {
			result = resolve(n.dependsOnKey)
		}
		delete(visiting, key)
		resolved[key] = result
		return result
	}

	for _, n := range nodes {
		resolve(n.key)
	}
	return resolved
}

// QuadrantOption es una opción del multiselect de filtro por cuadrante,
// combinando los slugs de Impacto/Esfuerzo y de Eisenhower.
type QuadrantOption struct {
	Value, Label string
}

func quadrantOptions() []QuadrantOption {
	return []QuadrantOption{
		{Value: scoring.QuadrantQuickWins, Label: scoring.ImpactEffortLabel(scoring.QuadrantQuickWins)},
		{Value: scoring.QuadrantMajorProjects, Label: scoring.ImpactEffortLabel(scoring.QuadrantMajorProjects)},
		{Value: scoring.QuadrantFillIns, Label: scoring.ImpactEffortLabel(scoring.QuadrantFillIns)},
		{Value: scoring.QuadrantThanklessTasks, Label: scoring.ImpactEffortLabel(scoring.QuadrantThanklessTasks)},
		{Value: scoring.EisenhowerDo, Label: scoring.EisenhowerLabel(scoring.EisenhowerDo)},
		{Value: scoring.EisenhowerSchedule, Label: scoring.EisenhowerLabel(scoring.EisenhowerSchedule)},
		{Value: scoring.EisenhowerDelegate, Label: scoring.EisenhowerLabel(scoring.EisenhowerDelegate)},
		{Value: scoring.EisenhowerDelete, Label: scoring.EisenhowerLabel(scoring.EisenhowerDelete)},
	}
}

func matchesQuadrantFilter(sc scoring.ActivityScores, quadrants []string) bool {
	if len(quadrants) == 0 {
		return true
	}
	for _, q := range quadrants {
		if sc.ImpactEffortQuadrant == q || sc.EisenhowerQuadrant == q {
			return true
		}
	}
	return false
}

// TimelineFilters son los filtros combinables del timeline, tal cual llegan
// (y se re-serializan) en los query params — así la URL resultante es
// compartible y recargable (§3 Fase 3, tarea 4).
type TimelineFilters struct {
	Sections  []string
	Quadrants []string
	From, To  string
}

func parseTimelineFilters(r *http.Request) TimelineFilters {
	q := r.URL.Query()
	return TimelineFilters{
		Sections:  q["section"],
		Quadrants: q["quadrant"],
		From:      q.Get("from"),
		To:        q.Get("to"),
	}
}

// TimelinePageData es el contexto de /timeline.
type TimelinePageData struct {
	Title, Nav        string
	Sections          []string
	QuadrantOptions   []QuadrantOption
	Filters           TimelineFilters
	SVG               svg.TimelineView
	Count, TotalCount int
}

// timelineRange calcula la escala temporal auto: min due − 7d, max due + 14d
// (§3 Fase 3, tarea 3), a partir de los entregables efectivamente mostrados.
func timelineRange(rows []svg.DeliverableRow, now time.Time) (time.Time, time.Time) {
	if len(rows) == 0 {
		return now.AddDate(0, 0, -7), now.AddDate(0, 0, 14)
	}
	min, max := rows[0].Due, rows[0].Due
	for _, r := range rows[1:] {
		if r.Due.Before(min) {
			min = r.Due
		}
		if r.Due.After(max) {
			max = r.Due
		}
	}
	return min.AddDate(0, 0, -7), max.AddDate(0, 0, 14)
}

func (s *Server) buildTimelinePageData(f TimelineFilters) TimelinePageData {
	snap := s.store.Snapshot()
	now := time.Now().UTC()

	nodes := buildDeliverableNodes(snap.Activities, now)
	atRisk := resolveAtRisk(nodes)

	sectionSet := make(map[string]bool, len(f.Sections))
	for _, sec := range f.Sections {
		sectionSet[sec] = true
	}
	var fromDate, toDate time.Time
	hasFrom, hasTo := false, false
	if f.From != "" {
		if t, err := time.Parse("2006-01-02", f.From); err == nil {
			fromDate, hasFrom = t, true
		}
	}
	if f.To != "" {
		if t, err := time.Parse("2006-01-02", f.To); err == nil {
			toDate, hasTo = t, true
		}
	}

	activityByID := make(map[string]model.Activity, len(snap.Activities))
	scoresByID := make(map[string]scoring.ActivityScores, len(snap.Activities))
	for _, a := range snap.Activities {
		activityByID[a.ID] = a
		scoresByID[a.ID] = scoring.Compute(a, snap.Weights, snap.Thresholds, now)
	}

	var rows []svg.DeliverableRow
	for _, n := range nodes {
		a := activityByID[n.activityID]
		if len(sectionSet) > 0 && !sectionSet[a.Section] {
			continue
		}
		if !matchesQuadrantFilter(scoresByID[n.activityID], f.Quadrants) {
			continue
		}
		if hasFrom && n.due.Before(fromDate) {
			continue
		}
		if hasTo && n.due.After(toDate) {
			continue
		}
		rows = append(rows, svg.DeliverableRow{
			Key: n.key, ActivityName: n.activityName, Name: n.name, Status: n.status,
			Due: n.due, AtRisk: atRisk[n.key], DependsOnKey: n.dependsOnKey,
			DetailURL: "/matrix/impact-effort/detail/" + n.activityID,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ActivityName != rows[j].ActivityName {
			return rows[i].ActivityName < rows[j].ActivityName
		}
		return rows[i].Due.Before(rows[j].Due)
	})

	minDate, maxDate := timelineRange(rows, now)

	return TimelinePageData{
		Title: "Timeline de entregables", Nav: "timeline",
		Sections:        snap.Sections,
		QuadrantOptions: quadrantOptions(),
		Filters:         f,
		SVG:             svg.BuildTimeline(rows, minDate, maxDate, now, "Timeline de entregables"),
		Count:           len(rows),
		TotalCount:      len(nodes),
	}
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	data := s.buildTimelinePageData(parseTimelineFilters(r))
	s.renderFull(w, "timeline", data)
}

func (s *Server) handleTimelineSVG(w http.ResponseWriter, r *http.Request) {
	data := s.buildTimelinePageData(parseTimelineFilters(r))
	s.renderFragment(w, "timeline", "timeline_container", data)
}
