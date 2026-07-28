package svg

// PointInput es un punto a plotear, en coordenadas de score (0-100).
type PointInput struct {
	ID        string
	X, Y      float64
	R         float64
	Color     string
	Label     string
	Dashed    bool
	Badge     string
	DetailURL string
}

// RenderedPoint es un punto ya proyectado a píxeles, listo para el template
// (que no debe hacer ninguna aritmética).
type RenderedPoint struct {
	ID, Color, Label, Badge, DetailURL string
	CX, CY, R                          float64
	Dashed                             bool
	BadgeX, BadgeY                     float64
}

// MatrixConfig son los parámetros de una matriz 2x2 genérica: ejes,
// umbrales (en score 0-100) y etiquetas de cuadrante.
type MatrixConfig struct {
	XLabel, YLabel                          string
	ThresholdX, ThresholdY                  float64
	QuadrantTopLeft, QuadrantTopRight       string
	QuadrantBottomLeft, QuadrantBottomRight string
	AriaLabel                               string
}

// MatrixView es la vista completamente precomputada que consume la
// plantilla matrix_svg.svg.tmpl (helper de render común a ambas matrices).
type MatrixView struct {
	Width, Height              float64
	PadLeft, PadTop            float64
	PlotRight, PlotBottom      float64
	ThresholdXpx, ThresholdYpx float64
	CenterX, CenterY           float64
	XLabel, YLabel, AriaLabel  string

	QuadrantTopLeft, QuadrantTopRight       string
	QuadrantBottomLeft, QuadrantBottomRight string
	QLabelTopY, QLabelBottomY               float64
	QLabelLeftX, QLabelRightX               float64

	Points []RenderedPoint
}

// BuildMatrix proyecta cfg + points a una MatrixView lista para renderizar.
func BuildMatrix(cfg MatrixConfig, points []PointInput) MatrixView {
	v := MatrixView{
		Width: Width, Height: Height,
		PadLeft: PadLeft, PadTop: PadTop,
		PlotRight:  Width - PadRight,
		PlotBottom: Height - PadBottom,

		ThresholdXpx: X(cfg.ThresholdX),
		ThresholdYpx: Y(cfg.ThresholdY),

		CenterX: PadLeft + plotWidth()/2,
		CenterY: PadTop + plotHeight()/2,

		XLabel: cfg.XLabel, YLabel: cfg.YLabel, AriaLabel: cfg.AriaLabel,

		QuadrantTopLeft:     cfg.QuadrantTopLeft,
		QuadrantTopRight:    cfg.QuadrantTopRight,
		QuadrantBottomLeft:  cfg.QuadrantBottomLeft,
		QuadrantBottomRight: cfg.QuadrantBottomRight,

		QLabelTopY:    PadTop + 16,
		QLabelBottomY: Height - PadBottom - 8,
		QLabelLeftX:   PadLeft + labelInset,
		QLabelRightX:  Width - PadRight - labelInset,
	}

	v.Points = make([]RenderedPoint, 0, len(points))
	for _, p := range points {
		cx, cy := X(p.X), Y(p.Y)
		v.Points = append(v.Points, RenderedPoint{
			ID:        p.ID,
			Color:     p.Color,
			Label:     p.Label,
			Badge:     p.Badge,
			DetailURL: p.DetailURL,
			CX:        cx,
			CY:        cy,
			R:         p.R,
			Dashed:    p.Dashed,
			BadgeX:    cx + p.R*badgeOffset,
			BadgeY:    cy - p.R*badgeOffset,
		})
	}
	return v
}
