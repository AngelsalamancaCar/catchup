package server

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"projectmapper/internal/model"
	"projectmapper/internal/scoring"
	"projectmapper/internal/svg"
)

// exportDir es el directorio (relativo al directorio de trabajo del
// binario) donde se guarda copia de cada export — §3 Fase 4.
const exportDir = "export"

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// ---------- Export XLSX (§3 Fase 4, tarea 1) ----------

var activitiesHeader = []any{
	"ID", "Nombre", "Tipo", "Owner", "Sección", "Involucradas", "Descripción",
	"B1", "B2", "B3", "B4", "C1", "C2", "C3", "C4", "D1", "D2", "D3", "D4",
	"F1", "F2", "F3",
	"Impacto", "Esfuerzo", "Urgencia", "Importancia", "Incertidumbre", "InfoPriority",
	"CuadranteImpactoEsfuerzo", "CuadranteEisenhower", "NecesitaInfo", "DecideConLoQueTienes",
}

func writeActivitiesSheet(f *excelize.File, snap model.Store, now time.Time) error {
	const sheet = "Actividades"
	if err := f.SetSheetRow(sheet, "A1", &activitiesHeader); err != nil {
		return err
	}
	for i, a := range snap.Activities {
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		d1 := ""
		if a.Answers.D1 != nil {
			d1 = *a.Answers.D1
		}
		row := []any{
			a.ID, a.Name, a.Type, a.Owner, a.Section, strings.Join(a.Involved, "; "), a.Description,
			a.Answers.B1, a.Answers.B2, a.Answers.B3, a.Answers.B4,
			a.Answers.C1, a.Answers.C2, a.Answers.C3, a.Answers.C4,
			d1, a.Answers.D2, a.Answers.D3, a.Answers.D4,
			a.Answers.F1, strings.Join(a.Answers.F2, "; "), a.Answers.F3,
			round1(sc.Impact.Score), round1(sc.Effort.Score), round1(sc.Urgency.Score), round1(sc.Importance.Score),
			round1(sc.Uncertainty.Score), round1(sc.InfoPriority),
			scoring.ImpactEffortLabel(sc.ImpactEffortQuadrant), scoring.EisenhowerLabel(sc.EisenhowerQuadrant),
			sc.NeedsInfo, sc.DecideWithWhatYouHave,
		}
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return err
		}
	}
	return f.SetColWidth(sheet, "B", "B", 30)
}

var deliverablesHeader = []any{
	"ActividadID", "Actividad", "DeliverableID", "Nombre", "Fecha", "Estado", "DependeDe", "AtRisk",
}

func writeDeliverablesSheet(f *excelize.File, snap model.Store, now time.Time) error {
	const sheet = "Deliverables"
	if err := f.SetSheetRow(sheet, "A1", &deliverablesHeader); err != nil {
		return err
	}
	atRisk := resolveAtRisk(buildDeliverableNodes(snap.Activities, now))

	row := 2
	for _, a := range snap.Activities {
		for _, d := range a.Deliverables {
			dependsOn := ""
			if d.DependsOn != nil {
				dependsOn = *d.DependsOn
			}
			vals := []any{a.ID, a.Name, d.ID, d.Name, d.Due, d.Status, dependsOn, atRisk[deliverableKey(a.ID, d.ID)]}
			cell, err := excelize.CoordinatesToCellName(1, row)
			if err != nil {
				return err
			}
			if err := f.SetSheetRow(sheet, cell, &vals); err != nil {
				return err
			}
			row++
		}
	}
	return f.SetColWidth(sheet, "B", "D", 24)
}

// writeScatterSheet arma una hoja con una serie de actividades (X, Y) y un
// scatter chart nativo con dos series adicionales de solo-línea que dibujan
// las líneas de umbral (cruz de cuadrantes) — no hay soporte nativo de
// excelize para "líneas de referencia" sobre un scatter, así que se simulan
// con series de 2 puntos sin marcador.
func writeScatterSheet(f *excelize.File, sheet, xLabel, yLabel string, thresholdX, thresholdY float64, points []svg.PointInput, names []string) error {
	if err := f.SetSheetRow(sheet, "A1", &[]any{"Actividad", xLabel, yLabel}); err != nil {
		return err
	}
	for i, p := range points {
		row := []any{names[i], round1(p.X), round1(p.Y)}
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return err
		}
	}

	if err := f.SetSheetRow(sheet, "F1", &[]any{"UmbralYx", "UmbralYy"}); err != nil {
		return err
	}
	if err := f.SetSheetRow(sheet, "F2", &[]any{0, thresholdY}); err != nil {
		return err
	}
	if err := f.SetSheetRow(sheet, "F3", &[]any{100, thresholdY}); err != nil {
		return err
	}
	if err := f.SetSheetRow(sheet, "H1", &[]any{"UmbralXx", "UmbralXy"}); err != nil {
		return err
	}
	if err := f.SetSheetRow(sheet, "H2", &[]any{thresholdX, 0}); err != nil {
		return err
	}
	if err := f.SetSheetRow(sheet, "H3", &[]any{thresholdX, 100}); err != nil {
		return err
	}

	if len(points) == 0 {
		return nil // sin actividades no hay nada que graficar
	}
	dataRange := func(col string) string {
		return fmt.Sprintf("%s!$%s$2:$%s$%d", sheet, col, col, len(points)+1)
	}
	minY, maxY := 0.0, 100.0
	return f.AddChart(sheet, "K2", &excelize.Chart{
		Type: excelize.Scatter,
		Series: []excelize.ChartSeries{
			{Name: "Actividades", Categories: dataRange("B"), Values: dataRange("C"), Marker: excelize.ChartMarker{Symbol: "circle"}},
			{Name: "Umbral " + yLabel, Categories: sheet + "!$F$2:$F$3", Values: sheet + "!$G$2:$G$3", Marker: excelize.ChartMarker{Symbol: "none"}, Line: excelize.LineOptions{Width: 1}},
			{Name: "Umbral " + xLabel, Categories: sheet + "!$H$2:$H$3", Values: sheet + "!$I$2:$I$3", Marker: excelize.ChartMarker{Symbol: "none"}, Line: excelize.LineOptions{Width: 1}},
		},
		Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: yLabel + " vs " + xLabel}}},
		XAxis: excelize.ChartAxis{Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: xLabel}}}, Minimum: &minY, Maximum: &maxY},
		YAxis: excelize.ChartAxis{Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: yLabel}}}, Minimum: &minY, Maximum: &maxY},
	})
}

func writeImpactEffortSheet(f *excelize.File, snap model.Store, now time.Time) error {
	var points []svg.PointInput
	var names []string
	for _, a := range snap.Activities {
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		points = append(points, svg.PointInput{X: sc.Effort.Score, Y: sc.Impact.Score})
		names = append(names, a.Name)
	}
	return writeScatterSheet(f, "Matriz IE", "Esfuerzo", "Impacto", snap.Thresholds.Effort, snap.Thresholds.Impact, points, names)
}

func writeEisenhowerSheet(f *excelize.File, snap model.Store, now time.Time) error {
	var points []svg.PointInput
	var names []string
	for _, a := range snap.Activities {
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		points = append(points, svg.PointInput{X: sc.Urgency.Score, Y: sc.Importance.Score})
		names = append(names, a.Name)
	}
	return writeScatterSheet(f, "Eisenhower", "Urgencia", "Importancia", snap.Thresholds.Urgency, snap.Thresholds.Importance, points, names)
}

var summaryHeader = []any{
	"Sección", "Actividades", "PersonaDíasTotales", "QuickWins", "MajorProjects", "FillIns", "ThanklessTasks",
	"ConectoresCrossSección", "RiesgoCoordinación", "CandidataRevisión",
}

type sectionCounts struct {
	activities, quickWins, majorProjects, fillIns, thanklessTasks int
	personDays                                                    float64
}

func writeSectionSummarySheet(f *excelize.File, snap model.Store, now time.Time) error {
	const sheet = "Resumen"
	if err := f.SetSheetRow(sheet, "A1", &summaryHeader); err != nil {
		return err
	}

	diag := computeSectionDiagnostics(snap, now)
	bySection := map[string]*sectionCounts{}
	order := append([]string{}, snap.Sections...)
	for _, sec := range order {
		bySection[sec] = &sectionCounts{}
	}
	for _, a := range snap.Activities {
		sec := a.Section
		if isOrphanSection(snap.Sections, sec) {
			sec = svg.OrphanLane
			if _, ok := bySection[sec]; !ok {
				bySection[sec] = &sectionCounts{}
				order = append(order, sec)
			}
		}
		c := bySection[sec]
		c.activities++
		c.personDays += scoring.PersonDaysMidpoint(a.Answers.C1)
		sc := scoring.Compute(a, snap.Weights, snap.Thresholds, now)
		switch sc.ImpactEffortQuadrant {
		case scoring.QuadrantQuickWins:
			c.quickWins++
		case scoring.QuadrantMajorProjects:
			c.majorProjects++
		case scoring.QuadrantFillIns:
			c.fillIns++
		case scoring.QuadrantThanklessTasks:
			c.thanklessTasks++
		}
	}

	for i, sec := range order {
		c := bySection[sec]
		row := []any{
			sec, c.activities, round1(c.personDays), c.quickWins, c.majorProjects, c.fillIns, c.thanklessTasks,
			diag.crossLinks[sec], diag.crossLinks[sec] > coordinationRiskThreshold, c.thanklessTasks >= thanklessReviewThreshold,
		}
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return err
		}
	}
	return f.SetColWidth(sheet, "A", "A", 20)
}

// buildPortfolioWorkbook arma el .xlsx completo del portafolio (§3 Fase 4,
// tarea 1): Actividades, Deliverables, Matriz IE y Eisenhower (con scatter
// chart nativo + líneas de umbral) y Resumen por sección. Es puro respecto
// del Store (solo lee snap), para poder testearlo sin servidor HTTP.
func buildPortfolioWorkbook(snap model.Store, now time.Time) (*excelize.File, error) {
	f := excelize.NewFile()

	if err := f.SetSheetName("Sheet1", "Actividades"); err != nil {
		return nil, err
	}
	if err := writeActivitiesSheet(f, snap, now); err != nil {
		return nil, err
	}

	for _, step := range []struct {
		sheet string
		write func(*excelize.File, model.Store, time.Time) error
	}{
		{"Deliverables", writeDeliverablesSheet},
		{"Matriz IE", writeImpactEffortSheet},
		{"Eisenhower", writeEisenhowerSheet},
		{"Resumen", writeSectionSummarySheet},
	} {
		if _, err := f.NewSheet(step.sheet); err != nil {
			return nil, err
		}
		if err := step.write(f, snap, now); err != nil {
			return nil, err
		}
	}

	f.SetActiveSheet(0)
	return f, nil
}

func (s *Server) handleExportXLSX(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	now := time.Now().UTC()

	f, err := buildPortfolioWorkbook(snap, now)
	if err != nil {
		http.Error(w, "error generando xlsx: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("portfolio_%s.xlsx", now.Format("20060102"))
	if err := f.SaveAs(filepath.Join(exportDir, filename)); err != nil {
		http.Error(w, "error guardando xlsx: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if _, err := f.WriteTo(w); err != nil {
		log.Printf("export xlsx: escribir respuesta: %v", err)
	}
}

// ---------- Export SVG (§3 Fase 4, tarea 2) ----------

// renderSVGAttachment ejecuta el mismo template SVG usado en pantalla, pero
// como archivo standalone descargable (con prólogo XML y headers de
// descarga) — reutiliza el mismo grupo de templates que las vistas HTMX.
func (s *Server) renderSVGAttachment(w http.ResponseWriter, group, name, filename string, data any) {
	t, ok := s.tmpl[group]
	if !ok {
		http.Error(w, "template group desconocido: "+group, http.StatusInternalServerError)
		return
	}
	var buf strings.Builder
	buf.WriteString(xml.Header)
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("export svg (%s/%s): %v", group, name, err)
		http.Error(w, "error de render", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	fmt.Fprint(w, buf.String())
}

func (s *Server) handleExportSVG(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("view") {
	case "impact-effort":
		data := s.buildImpactEffortPageData(s.store.Thresholds(), "section", false).SVG
		s.renderSVGAttachment(w, "matrix_impact_effort", "matrix_svg", "impacto-esfuerzo.svg", data)
	case "eisenhower":
		data := s.buildEisenhowerPageData(s.store.Thresholds(), false).SVG
		s.renderSVGAttachment(w, "matrix_eisenhower", "matrix_svg", "eisenhower.svg", data)
	case "org-swimlanes":
		data := s.buildOrgSwimlanesPageData("").SVG
		s.renderSVGAttachment(w, "org_swimlanes", "swimlane_svg", "organizacion-swimlanes.svg", data)
	case "org-treemap":
		data := s.buildOrgTreemapPageData().SVG
		s.renderSVGAttachment(w, "org_treemap", "treemap_svg", "organizacion-treemap.svg", data)
	case "timeline":
		data := s.buildTimelinePageData(TimelineFilters{}).SVG
		s.renderSVGAttachment(w, "timeline", "timeline_svg", "timeline.svg", data)
	default:
		http.NotFound(w, r)
	}
}

// ---------- Backup JSON (§3 Fase 4, tarea 3) ----------

func (s *Server) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("backup_%s.json", time.Now().UTC().Format("20060102_150405"))
	if err := os.WriteFile(filepath.Join(exportDir, filename), b, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write(b)
}
