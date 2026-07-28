package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"projectmapper/internal/model"
	"projectmapper/internal/scoring"
	"projectmapper/internal/svg"
)

// ActivityDetailData es el contexto del panel de detalle que se abre al
// hacer click en un punto de cualquiera de las dos matrices (helper común).
type ActivityDetailData struct {
	Activity           model.Activity
	Scores             scoring.ActivityScores
	DelegateCandidates []string
}

// delegateCandidates lista owners de otras actividades de la misma sección
// como candidatos de delegación — §2.4 del plan.
func delegateCandidates(a model.Activity, all []model.Activity) []string {
	seen := map[string]bool{a.Owner: true}
	var out []string
	for _, o := range all {
		if o.ID == a.ID || o.Section != a.Section || o.Owner == "" || seen[o.Owner] {
			continue
		}
		seen[o.Owner] = true
		out = append(out, o.Owner)
	}
	return out
}

// ---------- Matriz Impacto/Esfuerzo ----------

// ImpactEffortPageData es el contexto de /matrix/impact-effort.
type ImpactEffortPageData struct {
	Title, Nav                       string
	Sections                         []string
	ImpactThreshold, EffortThreshold float64
	ColorMode                        string
	ShowAll                          bool
	SVG                              svg.MatrixView
	Count, TotalCount                int
}

// isImpactEffortType reporta si el tipo de actividad se destaca por defecto
// en la matriz Impacto/Esfuerzo (Projects/Workstreams) — A2, §4.3 propuesta.
func isImpactEffortType(t string) bool {
	return t == "Project" || t == "Workstream"
}

// isEisenhowerType reporta si el tipo de actividad se destaca por defecto en
// el Eisenhower (Recurring/Ad-hoc) — A2, §4.3 propuesta.
func isEisenhowerType(t string) bool {
	return t == "Recurring" || t == "AdHoc"
}

func (s *Server) impactEffortPoints(snap model.Store, th model.Thresholds, colorMode string, showAll bool, now time.Time) ([]svg.PointInput, int) {
	var points []svg.PointInput
	shown := 0
	for _, a := range snap.Activities {
		if !showAll && !isImpactEffortType(a.Type) {
			continue
		}
		shown++
		sc := scoring.Compute(a, snap.Weights, th, now)
		color := svg.SectionColor(snap.Sections, a.Section)
		if colorMode == "fit" {
			color = svg.StrategicFitColor(a.Answers.B1)
		}
		points = append(points, svg.PointInput{
			ID:        a.ID,
			X:         sc.Effort.Score,
			Y:         sc.Impact.Score,
			R:         svg.PointRadius(a.Answers.C1),
			Color:     color,
			Label:     fmt.Sprintf("%s — Impacto %.0f / Esfuerzo %.0f", a.Name, sc.Impact.Score, sc.Effort.Score),
			DetailURL: "/matrix/impact-effort/detail/" + a.ID,
		})
	}
	return points, shown
}

func (s *Server) buildImpactEffortPageData(th model.Thresholds, colorMode string, showAll bool) ImpactEffortPageData {
	snap := s.store.Snapshot()
	now := time.Now().UTC()
	points, shown := s.impactEffortPoints(snap, th, colorMode, showAll, now)

	cfg := svg.MatrixConfig{
		XLabel: "Esfuerzo →", YLabel: "Impacto →",
		ThresholdX: th.Effort, ThresholdY: th.Impact,
		QuadrantTopLeft: "Quick Wins", QuadrantTopRight: "Major Projects",
		QuadrantBottomLeft: "Fill-Ins", QuadrantBottomRight: "Thankless Tasks",
		AriaLabel: "Matriz de Impacto y Esfuerzo",
	}

	return ImpactEffortPageData{
		Title: "Matriz Impacto/Esfuerzo", Nav: "impact-effort",
		Sections:        snap.Sections,
		ImpactThreshold: th.Impact, EffortThreshold: th.Effort,
		ColorMode: colorMode, ShowAll: showAll,
		SVG:        svg.BuildMatrix(cfg, points),
		Count:      shown,
		TotalCount: len(snap.Activities),
	}
}

func (s *Server) handleMatrixImpactEffort(w http.ResponseWriter, r *http.Request) {
	data := s.buildImpactEffortPageData(s.store.Thresholds(), "section", false)
	s.renderFull(w, "matrix_impact_effort", data)
}

func (s *Server) handleMatrixImpactEffortSVG(w http.ResponseWriter, r *http.Request) {
	th := s.store.Thresholds()
	updated := false
	if v, err := strconv.ParseFloat(r.URL.Query().Get("impact_threshold"), 64); err == nil {
		th.Impact = v
		updated = true
	}
	if v, err := strconv.ParseFloat(r.URL.Query().Get("effort_threshold"), 64); err == nil {
		th.Effort = v
		updated = true
	}
	if updated {
		if err := s.store.SetThresholds(th); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	colorMode := r.URL.Query().Get("color_mode")
	if colorMode != "fit" {
		colorMode = "section"
	}
	showAll := r.URL.Query().Get("show_all") == "1"

	data := s.buildImpactEffortPageData(th, colorMode, showAll)
	s.renderFragment(w, "matrix_impact_effort", "matrix_container", data)
}

func (s *Server) handleMatrixImpactEffortDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := s.store.GetActivity(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	snap := s.store.Snapshot()
	sc := scoring.Compute(a, snap.Weights, snap.Thresholds, time.Now().UTC())
	s.renderFragment(w, "matrix_impact_effort", "detail_panel", ActivityDetailData{Activity: a, Scores: sc})
}

// ---------- Matriz Eisenhower ----------

// InfoRankedItem es una fila del panel "necesita información".
type InfoRankedItem struct {
	ID, Name     string
	InfoPriority float64
	F3           string
}

// DecideItem es una fila del panel "decide con lo que tienes".
type DecideItem struct{ ID, Name string }

// BorderlineItem es una fila del panel "en la frontera".
type BorderlineItem struct{ ID, Name, Axis string }

// EisenhowerPageData es el contexto de /matrix/eisenhower.
type EisenhowerPageData struct {
	Title, Nav                            string
	Sections                              []string
	UrgencyThreshold, ImportanceThreshold float64
	ShowAll                               bool
	SVG                                   svg.MatrixView
	Count, TotalCount                     int
	InfoRanked                            []InfoRankedItem
	DecideList                            []DecideItem
	BorderlineList                        []BorderlineItem
}

func (s *Server) buildEisenhowerPageData(th model.Thresholds, showAll bool) EisenhowerPageData {
	snap := s.store.Snapshot()
	now := time.Now().UTC()

	var points []svg.PointInput
	var infoRanked []InfoRankedItem
	var decideList []DecideItem
	var borderline []BorderlineItem
	shown := 0

	for _, a := range snap.Activities {
		if !showAll && !isEisenhowerType(a.Type) {
			continue
		}
		shown++
		sc := scoring.Compute(a, snap.Weights, th, now)

		badge := ""
		if sc.NeedsInfo {
			badge = "ℹ"
		}
		points = append(points, svg.PointInput{
			ID:    a.ID,
			X:     sc.Urgency.Score,
			Y:     sc.Importance.Score,
			R:     svg.PointRadius(a.Answers.C1),
			Color: svg.SectionColor(snap.Sections, a.Section),
			Label: fmt.Sprintf("%s — %s (Urgencia %.0f / Importancia %.0f)",
				a.Name, scoring.EisenhowerLabel(sc.EisenhowerQuadrant), sc.Urgency.Score, sc.Importance.Score),
			Dashed:    sc.NeedsInfo,
			Badge:     badge,
			DetailURL: "/matrix/eisenhower/detail/" + a.ID,
		})

		if sc.NeedsInfo {
			infoRanked = append(infoRanked, InfoRankedItem{ID: a.ID, Name: a.Name, InfoPriority: sc.InfoPriority, F3: a.Answers.F3})
		}
		if sc.DecideWithWhatYouHave {
			decideList = append(decideList, DecideItem{ID: a.ID, Name: a.Name})
		}
		if sc.UrgencyBorderline {
			borderline = append(borderline, BorderlineItem{ID: a.ID, Name: a.Name, Axis: "urgencia"})
		}
		if sc.ImportanceBorderline {
			borderline = append(borderline, BorderlineItem{ID: a.ID, Name: a.Name, Axis: "importancia"})
		}
	}

	sort.Slice(infoRanked, func(i, j int) bool { return infoRanked[i].InfoPriority > infoRanked[j].InfoPriority })

	cfg := svg.MatrixConfig{
		XLabel: "Urgencia →", YLabel: "Importancia →",
		ThresholdX: th.Urgency, ThresholdY: th.Importance,
		QuadrantTopLeft: "Agendar", QuadrantTopRight: "Hacer",
		QuadrantBottomLeft: "Eliminar", QuadrantBottomRight: "Delegar",
		AriaLabel: "Matriz Eisenhower",
	}

	return EisenhowerPageData{
		Title: "Matriz Eisenhower", Nav: "eisenhower",
		Sections:         snap.Sections,
		UrgencyThreshold: th.Urgency, ImportanceThreshold: th.Importance,
		ShowAll:        showAll,
		SVG:            svg.BuildMatrix(cfg, points),
		Count:          shown,
		TotalCount:     len(snap.Activities),
		InfoRanked:     infoRanked,
		DecideList:     decideList,
		BorderlineList: borderline,
	}
}

func (s *Server) handleMatrixEisenhower(w http.ResponseWriter, r *http.Request) {
	data := s.buildEisenhowerPageData(s.store.Thresholds(), false)
	s.renderFull(w, "matrix_eisenhower", data)
}

func (s *Server) handleMatrixEisenhowerSVG(w http.ResponseWriter, r *http.Request) {
	th := s.store.Thresholds()
	updated := false
	if v, err := strconv.ParseFloat(r.URL.Query().Get("urgency_threshold"), 64); err == nil {
		th.Urgency = v
		updated = true
	}
	if v, err := strconv.ParseFloat(r.URL.Query().Get("importance_threshold"), 64); err == nil {
		th.Importance = v
		updated = true
	}
	if updated {
		if err := s.store.SetThresholds(th); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	showAll := r.URL.Query().Get("show_all") == "1"

	data := s.buildEisenhowerPageData(th, showAll)
	s.renderFragment(w, "matrix_eisenhower", "matrix_container", data)
}

func (s *Server) handleMatrixEisenhowerDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := s.store.GetActivity(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	snap := s.store.Snapshot()
	sc := scoring.Compute(a, snap.Weights, snap.Thresholds, time.Now().UTC())

	data := ActivityDetailData{Activity: a, Scores: sc}
	if sc.EisenhowerQuadrant == scoring.EisenhowerDelegate && (a.Answers.D4 == "yes" || a.Answers.D4 == "partial") {
		data.DelegateCandidates = delegateCandidates(a, snap.Activities)
	}
	s.renderFragment(w, "matrix_eisenhower", "detail_panel", data)
}
