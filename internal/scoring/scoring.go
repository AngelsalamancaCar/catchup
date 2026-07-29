// Package scoring implementa el motor de puntuación de Project Mapper.
// Es puro (sin I/O): siempre calcula a partir de model.Answers + model.Weights,
// nunca lee ni escribe el store. Cada score expone su desglose (Breakdown)
// para trazabilidad, tal como exige §7 de la propuesta.
package scoring

import (
	"strconv"
	"strings"
	"time"

	"projectmapper/internal/model"
)

// Component es un término del desglose de un score: respuesta cruda, peso y
// contribución normalizada, para que la UI pueda mostrar "peso × respuesta".
type Component struct {
	Key          string
	Label        string
	RawValue     string
	Weight       float64
	Normalized   float64
	Contribution float64
}

// Result es un score 0-100 junto con su desglose auditable.
type Result struct {
	Score     float64
	Breakdown []Component
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

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// normScale5 normaliza una escala 1-5 a 0-100: (v-1)/4 × 100. Valores fuera
// de rango se capan (cubre también "C2 capado a 5").
func normScale5(v int) float64 {
	v = clampInt(v, 1, 5)
	return float64(v-1) / 4 * 100
}

// normC1 normaliza la banda de esfuerzo (persona-días) según §2.2 del plan.
func normC1(band string) float64 {
	switch band {
	case "<5":
		return 10
	case "5-20":
		return 30
	case "20-60":
		return 55
	case "60-120":
		return 80
	case ">120":
		return 100
	default:
		return 0
	}
}

// personDaysMidpoint son los puntos medios de persona-días por banda C1,
// usados para dimensionar el treemap organizacional — §2.4 del plan.
var personDaysMidpoint = map[string]float64{
	"<5":     2.5,
	"5-20":   12.5,
	"20-60":  40,
	"60-120": 90,
	">120":   150,
}

// PersonDaysMidpoint mapea la banda C1 a su punto medio de persona-días.
func PersonDaysMidpoint(band string) float64 {
	return personDaysMidpoint[band]
}

func normC4(v string) float64 {
	switch v {
	case "none":
		return 0
	case "some":
		return 50
	case "blocking":
		return 100
	default:
		return 0
	}
}

func normD2(v string) float64 {
	switch v {
	case "nothing":
		return 0
	case "friction":
		return 33
	case "escalation":
		return 66
	case "breach":
		return 100
	default:
		return 0
	}
}

func normD3(v string) float64 {
	switch v {
	case "mine":
		return 100
	case "shared":
		return 60
	case "other":
		return 20
	default:
		return 0
	}
}

// proximity calcula la proximidad de un deadline (§2.3): sin deadline→0;
// vencido u hoy→100; si no, rampa lineal que llega a 0 a partir de 60 días.
func proximity(d1 *string, now time.Time) float64 {
	if d1 == nil || *d1 == "" {
		return 0
	}
	due, err := time.Parse("2006-01-02", *d1)
	if err != nil {
		return 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dueDay := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
	daysRemaining := dueDay.Sub(today).Hours() / 24
	raw := 100 - daysRemaining*(100.0/60.0)
	return clamp(raw, 0, 100)
}

func weight(w map[string]float64, key string) float64 {
	if w == nil {
		return 0
	}
	return w[key]
}

func sumWeighted(comps []Component) Result {
	total := 0.0
	for i := range comps {
		comps[i].Contribution = comps[i].Weight * comps[i].Normalized
		total += comps[i].Contribution
	}
	return Result{Score: clamp(total, 0, 100), Breakdown: comps}
}

// ImpactScore = Σ (peso_Bi × norm(Bi)) — §2.3.
func ImpactScore(a model.Answers, w model.Weights) Result {
	comps := []Component{
		{Key: "B1", Label: "Contribución a objetivos estratégicos", RawValue: strconv.Itoa(a.B1), Weight: weight(w.Impact, "B1"), Normalized: normScale5(a.B1)},
		{Key: "B2", Label: "Stakeholders/usuarios afectados", RawValue: strconv.Itoa(a.B2), Weight: weight(w.Impact, "B2"), Normalized: normScale5(a.B2)},
		{Key: "B3", Label: "Efecto financiero o de valor", RawValue: strconv.Itoa(a.B3), Weight: weight(w.Impact, "B3"), Normalized: normScale5(a.B3)},
		{Key: "B4", Label: "Consecuencia de no hacerlo", RawValue: strconv.Itoa(a.B4), Weight: weight(w.Impact, "B4"), Normalized: normScale5(a.B4)},
	}
	return sumWeighted(comps)
}

// EffortScore = Σ (peso_Ci × norm(Ci)) — §2.3.
func EffortScore(a model.Answers, w model.Weights) Result {
	comps := []Component{
		{Key: "C1", Label: "Esfuerzo estimado (persona-días)", RawValue: a.C1, Weight: weight(w.Effort, "C1"), Normalized: normC1(a.C1)},
		{Key: "C2", Label: "Secciones/equipos que coordinan", RawValue: strconv.Itoa(a.C2), Weight: weight(w.Effort, "C2"), Normalized: normScale5(a.C2)},
		{Key: "C3", Label: "Complejidad técnica o procedural", RawValue: strconv.Itoa(a.C3), Weight: weight(w.Effort, "C3"), Normalized: normScale5(a.C3)},
		{Key: "C4", Label: "Dependencias externas", RawValue: a.C4, Weight: weight(w.Effort, "C4"), Normalized: normC4(a.C4)},
	}
	return sumWeighted(comps)
}

// UrgencyScore = 0.6 × proximidad(D1) + 0.4 × norm(D2) — §2.3.
func UrgencyScore(a model.Answers, now time.Time) Result {
	prox := proximity(a.D1, now)
	d1Raw := "sin deadline"
	if a.D1 != nil && *a.D1 != "" {
		d1Raw = *a.D1
	}
	comps := []Component{
		{Key: "D1", Label: "Proximidad del deadline", RawValue: d1Raw, Weight: 0.6, Normalized: prox},
		{Key: "D2", Label: "Costo de retrasar 30 días", RawValue: a.D2, Weight: 0.4, Normalized: normD2(a.D2)},
	}
	return sumWeighted(comps)
}

// ImportanceScore = 0.6 × norm(B1) + 0.4 × norm(D3) — §2.3.
func ImportanceScore(a model.Answers) Result {
	comps := []Component{
		{Key: "B1", Label: "Contribución a objetivos estratégicos", RawValue: strconv.Itoa(a.B1), Weight: 0.6, Normalized: normScale5(a.B1)},
		{Key: "D3", Label: "Propiedad del objetivo", RawValue: a.D3, Weight: 0.4, Normalized: normD3(a.D3)},
	}
	return sumWeighted(comps)
}

// UncertaintyScore = (5-F1)/4×100, +10 por cada clave en F2, cap 100 — §2.3.
func UncertaintyScore(a model.Answers) Result {
	base := normScale5(6 - clampInt(a.F1, 1, 5)) // (5-F1)/4*100 reescrito con normScale5: ver nota abajo
	// normScale5(6-F1) = ((6-F1)-1)/4*100 = (5-F1)/4*100 — coincide exactamente con la fórmula del plan.
	guessBonus := float64(len(a.F2)) * 10
	total := clamp(base+guessBonus, 0, 100)
	comps := []Component{
		{Key: "F1", Label: "Confianza en las estimaciones", RawValue: strconv.Itoa(a.F1), Weight: 1, Normalized: base, Contribution: base},
		{Key: "F2", Label: "Respuestas marcadas como conjetura", RawValue: strings.Join(a.F2, ", "), Weight: 1, Normalized: guessBonus, Contribution: guessBonus},
	}
	return Result{Score: total, Breakdown: comps}
}

// InfoPriorityScore = Importance × Uncertainty / 100 — rankea el panel
// "necesita información" del Eisenhower.
func InfoPriorityScore(importance, uncertainty float64) float64 {
	return importance * uncertainty / 100
}

// Quadrant slugs de la matriz Impacto/Esfuerzo.
const (
	QuadrantQuickWins      = "quick_wins"
	QuadrantMajorProjects  = "major_projects"
	QuadrantFillIns        = "fill_ins"
	QuadrantThanklessTasks = "thankless_tasks"
)

// ClassifyImpactEffort clasifica una actividad en uno de los 4 cuadrantes de
// la matriz Impacto/Esfuerzo según los umbrales configurados — §2.4.
func ClassifyImpactEffort(impact, effort float64, t model.Thresholds) string {
	highImpact := impact >= t.Impact
	highEffort := effort >= t.Effort
	switch {
	case highImpact && !highEffort:
		return QuadrantQuickWins
	case highImpact && highEffort:
		return QuadrantMajorProjects
	case !highImpact && !highEffort:
		return QuadrantFillIns
	default:
		return QuadrantThanklessTasks
	}
}

// Quadrant slugs del Eisenhower.
const (
	EisenhowerDo       = "do"
	EisenhowerSchedule = "schedule"
	EisenhowerDelegate = "delegate"
	EisenhowerDelete   = "delete"
)

// ClassifyEisenhower clasifica una actividad en Hacer/Agendar/Delegar/Eliminar
// según urgencia/importancia vs los umbrales configurados — §2.4.
func ClassifyEisenhower(urgency, importance float64, t model.Thresholds) string {
	highUrgency := urgency >= t.Urgency
	highImportance := importance >= t.Importance
	switch {
	case highUrgency && highImportance:
		return EisenhowerDo
	case !highUrgency && highImportance:
		return EisenhowerSchedule
	case highUrgency && !highImportance:
		return EisenhowerDelegate
	default:
		return EisenhowerDelete
	}
}

// borderlineBand es la tolerancia ±8 alrededor de un umbral — §2.4.
const borderlineBand = 8

// Borderline reporta si score está "en la frontera" de threshold (±8).
func Borderline(score, threshold float64) bool {
	d := score - threshold
	if d < 0 {
		d = -d
	}
	return d <= borderlineBand
}

// ActivityScores agrupa todos los scores y clasificaciones computados para
// una actividad — es lo que consumen los handlers de las matrices.
type ActivityScores struct {
	ActivityID string

	Impact      Result
	Effort      Result
	Urgency     Result
	Importance  Result
	Uncertainty Result

	InfoPriority float64

	ImpactEffortQuadrant string
	EisenhowerQuadrant   string

	ImpactBorderline     bool
	EffortBorderline     bool
	UrgencyBorderline    bool
	ImportanceBorderline bool

	// NeedsInfo: Uncertainty ≥ 50 o F2 no vacío → overlay "necesita info" — §2.4.
	NeedsInfo bool
	// DecideWithWhatYouHave: Uncertainty < 25 y Importance < 40 — §2.4.
	DecideWithWhatYouHave bool
}

// Compute calcula todos los scores y clasificaciones de una actividad.
func Compute(a model.Activity, w model.Weights, t model.Thresholds, now time.Time) ActivityScores {
	impact := ImpactScore(a.Answers, w)
	effort := EffortScore(a.Answers, w)
	urgency := UrgencyScore(a.Answers, now)
	importance := ImportanceScore(a.Answers)
	uncertainty := UncertaintyScore(a.Answers)
	infoPriority := InfoPriorityScore(importance.Score, uncertainty.Score)

	return ActivityScores{
		ActivityID:  a.ID,
		Impact:      impact,
		Effort:      effort,
		Urgency:     urgency,
		Importance:  importance,
		Uncertainty: uncertainty,

		InfoPriority: infoPriority,

		ImpactEffortQuadrant: ClassifyImpactEffort(impact.Score, effort.Score, t),
		EisenhowerQuadrant:   ClassifyEisenhower(urgency.Score, importance.Score, t),

		ImpactBorderline:     Borderline(impact.Score, t.Impact),
		EffortBorderline:     Borderline(effort.Score, t.Effort),
		UrgencyBorderline:    Borderline(urgency.Score, t.Urgency),
		ImportanceBorderline: Borderline(importance.Score, t.Importance),

		NeedsInfo:             uncertainty.Score >= 50 || len(a.Answers.F2) > 0,
		DecideWithWhatYouHave: uncertainty.Score < 25 && importance.Score < 40,
	}
}

// ImpactEffortLabel traduce el slug de cuadrante I/E a la etiqueta de UI.
func ImpactEffortLabel(quadrant string) string {
	switch quadrant {
	case QuadrantQuickWins:
		return "Quick Wins"
	case QuadrantMajorProjects:
		return "Major Projects"
	case QuadrantFillIns:
		return "Fill-Ins"
	case QuadrantThanklessTasks:
		return "Thankless Tasks"
	default:
		return quadrant
	}
}

// EisenhowerLabel traduce el slug de cuadrante Eisenhower a la acción en español.
func EisenhowerLabel(quadrant string) string {
	switch quadrant {
	case EisenhowerDo:
		return "Hacer"
	case EisenhowerSchedule:
		return "Agendar"
	case EisenhowerDelegate:
		return "Delegar"
	case EisenhowerDelete:
		return "Eliminar"
	default:
		return quadrant
	}
}
