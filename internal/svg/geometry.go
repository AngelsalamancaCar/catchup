// Package svg contiene los helpers de geometría compartidos por las
// matrices SVG (Impacto/Esfuerzo y Eisenhower). Es puro (sin I/O): solo
// mapea scores 0-100 a coordenadas de píxeles y arma la vista lista para
// que la plantilla la interpole sin hacer aritmética.
package svg

// Dimensiones del lienzo y márgenes del área de ploteo, en unidades de
// viewBox (equivalentes a px).
const (
	Width       = 640.0
	Height      = 460.0
	PadLeft     = 55.0
	PadRight    = 20.0
	PadTop      = 20.0
	PadBottom   = 45.0
	labelInset  = 10.0
	badgeOffset = 0.6
)

func plotWidth() float64  { return Width - PadLeft - PadRight }
func plotHeight() float64 { return Height - PadTop - PadBottom }

func clamp01(score float64) float64 {
	v := score / 100
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// X mapea un score 0-100 a la coordenada horizontal de píxel.
func X(score float64) float64 {
	return PadLeft + clamp01(score)*plotWidth()
}

// Y mapea un score 0-100 a la coordenada vertical de píxel (invertida:
// scores altos quedan arriba, como es convención en estas matrices).
func Y(score float64) float64 {
	return PadTop + (1-clamp01(score))*plotHeight()
}

// SectionPalette son los 8 colores fijos y accesibles para secciones
// organizacionales (coincide con --section-1..8 de app.css).
var SectionPalette = []string{
	"#0f6cbd", "#a4262c", "#038387", "#986f0b",
	"#5c2e91", "#0b6a0b", "#b4009e", "#605e5c",
}

const orphanColor = "#8a8886"

func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

// SectionColor devuelve el color de paleta asignado a section según su
// posición en la lista de secciones configuradas.
func SectionColor(sections []string, section string) string {
	idx := indexOf(sections, section)
	if idx < 0 {
		return orphanColor
	}
	return SectionPalette[idx%len(SectionPalette)]
}

// strategicFitScale es la escala secuencial clara→oscura para colorear por B1.
var strategicFitScale = []string{"#cfe4fa", "#96c6fa", "#479ef5", "#0f6cbd", "#0e4775"}

// StrategicFitColor devuelve un color en escala secuencial según B1 (1-5).
func StrategicFitColor(b1 int) string {
	idx := min(max(b1-1, 0), 4)
	return strategicFitScale[idx]
}

// bandRadius son los radios (px) asignados a cada banda de esfuerzo C1.
var bandRadius = map[string]float64{
	"<5":     5,
	"5-20":   7,
	"20-60":  9,
	"60-120": 11,
	">120":   13,
}

// PointRadius mapea la banda C1 (persona-días) a un radio de burbuja en px.
func PointRadius(band string) float64 {
	if r, ok := bandRadius[band]; ok {
		return r
	}
	return 6
}
