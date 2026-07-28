package server

import (
	"fmt"
	"html/template"
	"slices"
	"strings"
	"time"

	"projectmapper/internal/scoring"
	"projectmapper/internal/svg"
)

// ScaleOption es un punto de una escala 1-5 con su ancla textual.
type ScaleOption struct {
	Value  int
	Anchor string
}

// anchors devuelve las descripciones ancla por punto de escala para cada
// pregunta 1-5 del cuestionario (mitigación de riesgo §8 de la propuesta:
// evita que el usuario "juegue" la escala sin criterio).
func anchors(question string) []ScaleOption {
	table := map[string][5]string{
		"b1": {
			"Sin relación con objetivos estratégicos",
			"Contribución indirecta",
			"Apoya un objetivo secundario",
			"Apoya un objetivo estratégico principal",
			"Habilita directamente un objetivo top",
		},
		"b2": {
			"Solo yo",
			"Un equipo pequeño",
			"Una sección completa",
			"Varias secciones",
			"Toda la organización",
		},
		"b3": {
			"Insignificante",
			"Bajo",
			"Moderado",
			"Alto",
			"Crítico (cifra mayor / riesgo grande)",
		},
		"b4": {
			"Ninguna consecuencia",
			"Molestia menor",
			"Pérdida de oportunidad",
			"Daño significativo",
			"Consecuencia grave o irreversible",
		},
		"c2": {
			"Solo mi sección",
			"2 secciones",
			"3 secciones",
			"4 secciones",
			"5 o más secciones",
		},
		"c3": {
			"Trivial",
			"Simple",
			"Moderada",
			"Compleja",
			"Muy compleja / territorio desconocido",
		},
		"f1": {
			"Pura conjetura",
			"Poca confianza",
			"Confianza media",
			"Buena confianza",
			"Certeza alta",
		},
	}
	anchors, ok := table[question]
	if !ok {
		return nil
	}
	out := make([]ScaleOption, 5)
	for i := range 5 {
		out[i] = ScaleOption{Value: i + 1, Anchor: anchors[i]}
	}
	return out
}

// dict construye un map[string]any a partir de pares clave/valor, para pasar
// varios parámetros a un sub-template.
func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict: número impar de argumentos")
	}
	m := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: la clave %v no es string", values[i])
		}
		m[key] = values[i+1]
	}
	return m, nil
}

// contains reporta si item está presente en list.
func contains(list []string, item string) bool {
	return slices.Contains(list, item)
}

// fmtDate formatea una fecha ISO (2006-01-02) para mostrarla en la UI.
func fmtDate(iso string) string {
	if iso == "" {
		return "—"
	}
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02/01/2006")
}

// pct formatea un score 0-100 con un decimal.
func pct(v float64) string {
	return fmt.Sprintf("%.0f", v)
}

// slice arma un []string a partir de argumentos variádicos, útil para
// iterar una lista literal desde el template.
func slice(items ...string) []string {
	return items
}

// d2Label traduce el valor crudo de D2 a una etiqueta legible.
func d2Label(v string) string {
	switch v {
	case "nothing":
		return "Nada"
	case "friction":
		return "Fricción menor"
	case "escalation":
		return "Escalamiento"
	case "breach":
		return "Incumplimiento o pérdida"
	default:
		return v
	}
}

// c4Label traduce el valor crudo de C4 a una etiqueta legible.
func c4Label(v string) string {
	switch v {
	case "none":
		return "Ninguna"
	case "some":
		return "Algunas"
	case "blocking":
		return "Bloqueantes"
	default:
		return v
	}
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"anchors":           anchors,
		"dict":              dict,
		"contains":          contains,
		"fmtDate":           fmtDate,
		"pct":               pct,
		"slice":             slice,
		"join":              strings.Join,
		"d2Label":           d2Label,
		"c4Label":           c4Label,
		"sectionColor":      svg.SectionColor,
		"impactEffortLabel": scoring.ImpactEffortLabel,
		"eisenhowerLabel":   scoring.EisenhowerLabel,
	}
}
