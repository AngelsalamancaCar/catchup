package svg

// Constantes de layout del treemap organizacional.
const (
	TreemapWidth  = 760.0
	TreemapHeight = 220.0
)

// SectionAreaInput es una sección con su tamaño (persona-días totales) y su
// color de paleta, para dimensionar el treemap.
type SectionAreaInput struct {
	Name, Color, DetailURL string
	Value                  float64
}

// RenderedRect es un rectángulo del treemap ya proyectado a píxeles.
type RenderedRect struct {
	Name, Color, DetailURL string
	Value                  float64
	X, Y, Width, Height    float64
	LabelX, LabelY         float64
}

// TreemapView es la vista completamente precomputada del treemap.
type TreemapView struct {
	Width, Height float64
	Rects         []RenderedRect
	AriaLabel     string
}

// BuildTreemap reparte TreemapWidth proporcionalmente al valor (persona-días)
// de cada sección: slice-and-dice de un solo eje, suficiente para <20
// secciones (§3 Fase 3 del plan; squarified no se justifica con el seed).
// Área = ancho × TreemapHeight constante, así el área queda directamente
// proporcional al valor y verificable a mano.
func BuildTreemap(sections []SectionAreaInput, ariaLabel string) TreemapView {
	view := TreemapView{Width: TreemapWidth, Height: TreemapHeight, AriaLabel: ariaLabel}

	total := 0.0
	for _, s := range sections {
		total += s.Value
	}
	if total <= 0 {
		return view
	}

	x := 0.0
	for _, s := range sections {
		w := TreemapWidth * s.Value / total
		view.Rects = append(view.Rects, RenderedRect{
			Name: s.Name, Color: s.Color, DetailURL: s.DetailURL, Value: s.Value,
			X: x, Y: 0, Width: w, Height: TreemapHeight,
			LabelX: x + w/2, LabelY: TreemapHeight / 2,
		})
		x += w
	}
	return view
}
