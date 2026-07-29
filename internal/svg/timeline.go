package svg

import (
	"fmt"
	"time"
)

// Constantes de layout del timeline Gantt.
const (
	TimelineWidth   = 860.0
	tlLabelWidth    = 260.0
	tlPadRight      = 20.0
	tlPadTop        = 30.0
	tlPadBottom     = 20.0
	tlRowHeight     = 26.0
	TimelineMarkerR = 6.0
)

// DeliverableRow es un entregable a ubicar en el timeline, ya resuelto por
// el handler (fecha parseada, at-risk calculado con propagación transitiva).
type DeliverableRow struct {
	Key          string // clave única: ActivityID + "#" + Deliverable.ID
	ActivityName string
	Name, Status string
	Due          time.Time
	AtRisk       bool
	DependsOnKey string
	DetailURL    string
}

// RenderedDeliverable es una fila del timeline ya proyectada a píxeles.
type RenderedDeliverable struct {
	Key, ActivityName, Name, Status, DetailURL string
	Y, MarkerX                                 float64
	AtRisk                                     bool
}

// AxisTick es una marca de fecha en el eje horizontal.
type AxisTick struct {
	X     float64
	Label string
}

// DependencyArrow conecta el marcador de un entregable con el de aquel del
// que depende (E4), como un path curvo.
type DependencyArrow struct {
	PathD  string
	AtRisk bool
}

// TimelineView es la vista completamente precomputada del Gantt que consume
// timeline_svg.svg.tmpl (no hace ninguna aritmética).
type TimelineView struct {
	Width, Height                            float64
	PlotLeft, PlotRight, PlotTop, PlotBottom float64
	HasToday                                 bool
	TodayX                                   float64
	Ticks                                    []AxisTick
	Rows                                     []RenderedDeliverable
	Arrows                                   []DependencyArrow
	AriaLabel                                string
}

func tlX(minDate, maxDate, d time.Time) float64 {
	plotLeft := tlLabelWidth
	plotWidth := TimelineWidth - tlLabelWidth - tlPadRight
	span := maxDate.Sub(minDate).Hours()
	if span <= 0 {
		return plotLeft
	}
	frac := d.Sub(minDate).Hours() / span
	frac = clamp(frac, 0, 1)
	return plotLeft + frac*plotWidth
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// BuildTimeline proyecta filas de entregables a coordenadas de píxel: escala
// temporal auto (minDate/maxDate ya resueltos por el handler según §3 Fase 3:
// min due − 7d, max due + 14d), línea de "hoy" si cae en rango, y flechas de
// dependencia entre marcadores (E4).
func BuildTimeline(rows []DeliverableRow, minDate, maxDate, now time.Time, ariaLabel string) TimelineView {
	view := TimelineView{
		Width:     TimelineWidth,
		PlotLeft:  tlLabelWidth,
		PlotRight: TimelineWidth - tlPadRight,
		PlotTop:   tlPadTop,
		AriaLabel: ariaLabel,
	}
	view.Height = tlPadTop + float64(len(rows))*tlRowHeight + tlPadBottom
	view.PlotBottom = view.Height - tlPadBottom

	if !now.Before(minDate) && !now.After(maxDate) {
		view.HasToday = true
		view.TodayX = tlX(minDate, maxDate, now)
	}

	rowY := make(map[string]float64, len(rows))
	markerX := make(map[string]float64, len(rows))

	for i, r := range rows {
		y := tlPadTop + float64(i)*tlRowHeight + tlRowHeight/2
		x := tlX(minDate, maxDate, r.Due)
		rowY[r.Key] = y
		markerX[r.Key] = x
		view.Rows = append(view.Rows, RenderedDeliverable{
			Key: r.Key, ActivityName: r.ActivityName, Name: r.Name, Status: r.Status,
			DetailURL: r.DetailURL, Y: y, MarkerX: x, AtRisk: r.AtRisk,
		})
	}

	const maxTicks = 8
	span := maxDate.Sub(minDate)
	if span > 0 {
		for i := 0; i <= maxTicks; i++ {
			d := minDate.Add(time.Duration(float64(span) * float64(i) / float64(maxTicks)))
			view.Ticks = append(view.Ticks, AxisTick{X: tlX(minDate, maxDate, d), Label: d.Format("02/01")})
		}
	}

	for _, r := range rows {
		if r.DependsOnKey == "" {
			continue
		}
		fromY, ok1 := rowY[r.DependsOnKey]
		fromX, ok2 := markerX[r.DependsOnKey]
		toY, ok3 := rowY[r.Key]
		toX, ok4 := markerX[r.Key]
		if !ok1 || !ok2 || !ok3 || !ok4 {
			continue
		}
		view.Arrows = append(view.Arrows, DependencyArrow{
			PathD: arrowPath(fromX, fromY, toX, toY), AtRisk: r.AtRisk,
		})
	}

	return view
}

func arrowPath(x1, y1, x2, y2 float64) string {
	midY := (y1 + y2) / 2
	return fmt.Sprintf("M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f", x1, y1, x1, midY, x2, midY, x2, y2)
}
