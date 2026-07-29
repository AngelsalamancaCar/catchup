package server

import (
	"testing"
	"time"

	"projectmapper/internal/model"
)

func actWith(id, section string, involved []string, c1 string, b1, b2, b3, b4, c2, c3 int, c4 string) model.Activity {
	return model.Activity{
		ID: id, Name: id, Section: section, Involved: involved,
		Answers: model.Answers{B1: b1, B2: b2, B3: b3, B4: b4, C1: c1, C2: c2, C3: c3, C4: c4},
	}
}

func TestComputeSectionDiagnosticsCrossLinksCountsBothEnds(t *testing.T) {
	snap := model.Store{
		Sections: []string{"Finanzas", "IT"},
		Weights:  model.DefaultWeights(),
		Activities: []model.Activity{
			actWith("ACT-001", "Finanzas", []string{"IT"}, "<5", 1, 1, 1, 1, 1, 1, "none"),
		},
	}
	diag := computeSectionDiagnostics(snap, time.Now().UTC())
	if diag.crossLinks["Finanzas"] != 1 || diag.crossLinks["IT"] != 1 {
		t.Fatalf("esperaba 1 conector en cada extremo, tengo Finanzas=%d IT=%d", diag.crossLinks["Finanzas"], diag.crossLinks["IT"])
	}
}

func TestComputeSectionDiagnosticsIgnoresSelfInvolvement(t *testing.T) {
	snap := model.Store{
		Sections: []string{"Finanzas"},
		Weights:  model.DefaultWeights(),
		Activities: []model.Activity{
			actWith("ACT-001", "Finanzas", []string{"Finanzas"}, "<5", 1, 1, 1, 1, 1, 1, "none"),
		},
	}
	diag := computeSectionDiagnostics(snap, time.Now().UTC())
	if diag.crossLinks["Finanzas"] != 0 {
		t.Errorf("un enlace a la propia sección no debería contar como cross-sección, tengo %d", diag.crossLinks["Finanzas"])
	}
}

func TestRiskSectionsThreshold(t *testing.T) {
	snap := model.Store{
		Sections: []string{"Finanzas", "IT", "Legal", "Operaciones"},
		Weights:  model.DefaultWeights(),
		Activities: []model.Activity{
			actWith("ACT-001", "Finanzas", []string{"IT"}, "<5", 1, 1, 1, 1, 1, 1, "none"),
			actWith("ACT-002", "Finanzas", []string{"Legal"}, "<5", 1, 1, 1, 1, 1, 1, "none"),
			actWith("ACT-003", "Finanzas", []string{"Operaciones"}, "<5", 1, 1, 1, 1, 1, 1, "none"),
			actWith("ACT-004", "Finanzas", []string{"IT"}, "<5", 1, 1, 1, 1, 1, 1, "none"),
		},
	}
	diag := computeSectionDiagnostics(snap, time.Now().UTC())
	risk := diag.riskSections(snap.Sections)
	if len(risk) != 1 || risk[0] != "Finanzas" {
		t.Fatalf("Finanzas tiene 4 conectores (>3), debería estar en riesgo: %v", risk)
	}
}

func TestThanklessReviewBadge(t *testing.T) {
	// Thankless Tasks = bajo impacto, alto esfuerzo (umbrales default 50/50).
	low, highEffort := 1, 5
	snap := model.Store{
		Sections:   []string{"Legal"},
		Weights:    model.DefaultWeights(),
		Thresholds: model.DefaultThresholds(),
		Activities: []model.Activity{
			actWith("ACT-001", "Legal", nil, ">120", low, low, low, low, highEffort, highEffort, "blocking"),
			actWith("ACT-002", "Legal", nil, ">120", low, low, low, low, highEffort, highEffort, "blocking"),
		},
	}
	diag := computeSectionDiagnostics(snap, time.Now().UTC())
	badges := diag.badgesFor(snap.Sections)
	if len(badges["Legal"]) == 0 {
		t.Fatalf("Legal tiene 2 Thankless Tasks, esperaba badge de revisión: %v", badges)
	}
}

func TestIsOrphanSection(t *testing.T) {
	sections := []string{"Finanzas", "IT"}
	if isOrphanSection(sections, "Finanzas") {
		t.Errorf("Finanzas está configurada, no debería ser huérfana")
	}
	if !isOrphanSection(sections, "Marketing") {
		t.Errorf("Marketing no está configurada, debería ser huérfana")
	}
}
