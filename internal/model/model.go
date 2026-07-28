// Package model define las estructuras de datos persistidas de Project Mapper.
package model

import "time"

// Store es la raíz del documento JSON único.
type Store struct {
	Sections   []string   `json:"sections"`
	Weights    Weights    `json:"weights"`
	Thresholds Thresholds `json:"thresholds"`
	Activities []Activity `json:"activities"`
}

// Weights contiene los pesos configurables de cada dimensión de scoring.
// Las claves de Impact son B1..B4 y las de Effort son C1..C4.
type Weights struct {
	Impact map[string]float64 `json:"impact"`
	Effort map[string]float64 `json:"effort"`
}

// Thresholds son los umbrales 0-100 usados para clasificar cuadrantes.
type Thresholds struct {
	Impact     float64 `json:"impact"`
	Effort     float64 `json:"effort"`
	Urgency    float64 `json:"urgency"`
	Importance float64 `json:"importance"`
}

// Activity representa una actividad/proyecto capturado vía cuestionario.
type Activity struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Type         string        `json:"type"` // Project|Workstream|Recurring|AdHoc
	Owner        string        `json:"owner"`
	Section      string        `json:"section"`
	Involved     []string      `json:"involved"`
	Description  string        `json:"description"`
	Answers      Answers       `json:"answers"`
	Deliverables []Deliverable `json:"deliverables"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// Answers contiene las respuestas crudas del cuestionario (secciones B-F).
// Los scores nunca se persisten: se recalculan siempre desde Answers + Weights.
type Answers struct {
	B1 int `json:"b1"` // 1-5, contribución a objetivos estratégicos
	B2 int `json:"b2"` // 1-5, nº de stakeholders/usuarios afectados
	B3 int `json:"b3"` // 1-5, efecto financiero o de valor
	B4 int `json:"b4"` // 1-5, consecuencia de no hacerlo

	C1 string `json:"c1"` // banda: "<5"|"5-20"|"20-60"|"60-120"|">120"
	C2 int    `json:"c2"` // 1-5, nº de secciones que coordinan
	C3 int    `json:"c3"` // 1-5, complejidad técnica/procedural
	C4 string `json:"c4"` // "none"|"some"|"blocking"

	D1 *string `json:"d1"` // fecha ISO o null (sin deadline)
	D2 string  `json:"d2"` // "nothing"|"friction"|"escalation"|"breach"
	D3 string  `json:"d3"` // "mine"|"shared"|"other"
	D4 string  `json:"d4"` // "yes"|"partial"|"no"

	F1 int      `json:"f1"` // 1-5, confianza en las estimaciones
	F2 []string `json:"f2"` // claves marcadas como conjetura, ej ["B3","C1"]
	F3 string   `json:"f3"` // texto libre: info faltante y quién la tiene
}

// Deliverable es un entregable ligado a una actividad (sección E).
type Deliverable struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Due       string  `json:"due"`    // fecha ISO 2006-01-02
	Status    string  `json:"status"` // not_started|in_progress|at_risk|done
	DependsOn *string `json:"depends_on"`
}

// DefaultSections son las secciones organizacionales iniciales.
func DefaultSections() []string {
	return []string{"Operaciones", "Finanzas", "Legal", "IT"}
}

// DefaultWeights son los pesos por defecto de impacto y esfuerzo (§2.3 del plan).
func DefaultWeights() Weights {
	return Weights{
		Impact: map[string]float64{"B1": 0.4, "B2": 0.2, "B3": 0.25, "B4": 0.15},
		Effort: map[string]float64{"C1": 0.4, "C2": 0.2, "C3": 0.25, "C4": 0.15},
	}
}

// DefaultThresholds son los umbrales default de clasificación (50/50).
func DefaultThresholds() Thresholds {
	return Thresholds{Impact: 50, Effort: 50, Urgency: 50, Importance: 50}
}

// NewStore crea un Store vacío con los valores default documentados en el plan.
func NewStore() Store {
	return Store{
		Sections:   DefaultSections(),
		Weights:    DefaultWeights(),
		Thresholds: DefaultThresholds(),
		Activities: []Activity{},
	}
}
