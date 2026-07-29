// Package svg contiene los helpers de geometría compartidos por las
// matrices SVG (Impacto/Esfuerzo y Eisenhower). Es puro (sin I/O): solo
// mapea scores 0-100 a coordenadas de píxeles y arma la vista lista para
// que la plantilla la interpole sin hacer aritmética.
package svg

import "strconv"

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

// SectionPalette son los 5 colores fijos para secciones organizacionales:
// el ciclo mono-accent accent-400 → neutral-400 → accent-600 → neutral-600
// → accent-800 del design system Nocturne (coincide con --section-1..5 de
// app.css).
var SectionPalette = []string{
	"#b5abfc", "#b2b6ca", "#796cbf", "#75798c", "#423a6a",
}

const orphanColor = "#595d6c"

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

// strategicFitScale es la escala secuencial clara→oscura para colorear por
// B1: sube de neutral claro a accent oscuro, mono-accent (sin tonos
// inventados) — coincide con FitPalette expuesto a los templates.
var strategicFitScale = []string{"#e4e7f5", "#b2b6ca", "#968ae0", "#796cbf", "#423a6a"}

// FitPalette expone la escala de fit estratégico (B1) para que la leyenda
// del template no duplique los hex a mano.
func FitPalette() []string {
	return strategicFitScale
}

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

// textOnDark y textOnLight son los tokens de texto Nocturne usados como
// contraste sobre un fill de color arbitrario (paleta de secciones).
const (
	textOnDark  = "#e9e9ed" // --color-text
	textOnLight = "#161826" // --color-bg
)

// TextOnColor devuelve el color de texto (claro u oscuro) legible sobre un
// fill hexadecimal dado, por luminancia relativa aproximada. Necesario
// porque SectionPalette mezcla tonos claros y oscuros de los ramps accent/
// neutral (mono-accent), así que un solo color de texto fijo no sirve para
// todos (usado por treemap y tarjetas organizacionales de sección).
func TextOnColor(hex string) string {
	r, g, b, ok := parseHexRGB(hex)
	if !ok {
		return textOnDark
	}
	brightness := (299*r + 587*g + 114*b) / 1000
	if brightness > 140 {
		return textOnLight
	}
	return textOnDark
}

func parseHexRGB(hex string) (r, g, b int, ok bool) {
	hex = trimHash(hex)
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	var vals [3]int
	for i := range 3 {
		v, err := hexByte(hex[i*2 : i*2+2])
		if err != nil {
			return 0, 0, 0, false
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], true
}

func trimHash(s string) string {
	if len(s) > 0 && s[0] == '#' {
		return s[1:]
	}
	return s
}

func hexByte(s string) (int, error) {
	v, err := strconv.ParseUint(s, 16, 16)
	return int(v), err
}
