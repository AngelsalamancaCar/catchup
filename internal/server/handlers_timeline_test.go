package server

import (
	"testing"
	"time"

	"projectmapper/internal/model"
	"projectmapper/internal/scoring"
)

func strp(s string) *string { return &s }

func TestBuildDeliverableNodesSkipsInvalidDate(t *testing.T) {
	activities := []model.Activity{
		{ID: "ACT-001", Deliverables: []model.Deliverable{
			{ID: "DLV-001", Name: "válido", Due: "2026-08-01", Status: "in_progress"},
			{ID: "DLV-002", Name: "corrupto", Due: "no-es-fecha", Status: "in_progress"},
		}},
	}
	nodes := buildDeliverableNodes(activities, time.Now().UTC())
	if len(nodes) != 1 {
		t.Fatalf("esperaba 1 nodo (el corrupto se descarta), tengo %d", len(nodes))
	}
}

func TestResolveAtRiskBaseCase(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	activities := []model.Activity{
		{ID: "ACT-001", Deliverables: []model.Deliverable{
			{ID: "DLV-001", Name: "vence pronto", Due: "2026-08-10", Status: "in_progress"},
			{ID: "DLV-002", Name: "lejos", Due: "2026-12-01", Status: "in_progress"},
			{ID: "DLV-003", Name: "vencido pero done", Due: "2026-08-05", Status: "done"},
		}},
	}
	nodes := buildDeliverableNodes(activities, now)
	atRisk := resolveAtRisk(nodes)

	if !atRisk[deliverableKey("ACT-001", "DLV-001")] {
		t.Errorf("entregable que vence en <14 días y no está done debería ser at-risk")
	}
	if atRisk[deliverableKey("ACT-001", "DLV-002")] {
		t.Errorf("entregable lejano no debería ser at-risk")
	}
	if atRisk[deliverableKey("ACT-001", "DLV-003")] {
		t.Errorf("entregable done no debería ser at-risk aunque la fecha ya pasó")
	}
}

func TestResolveAtRiskPropagatesTransitively(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	activities := []model.Activity{
		{ID: "ACT-001", Deliverables: []model.Deliverable{
			{ID: "DLV-001", Name: "base at-risk", Due: "2026-08-05", Status: "in_progress"},
			{ID: "DLV-002", Name: "depende de 001", Due: "2026-12-01", Status: "in_progress", DependsOn: strp("DLV-001")},
			{ID: "DLV-003", Name: "depende de 002", Due: "2026-12-15", Status: "in_progress", DependsOn: strp("DLV-002")},
		}},
	}
	nodes := buildDeliverableNodes(activities, now)
	atRisk := resolveAtRisk(nodes)

	if !atRisk[deliverableKey("ACT-001", "DLV-002")] {
		t.Errorf("DLV-002 depende de un at-risk, debería heredar at-risk")
	}
	if !atRisk[deliverableKey("ACT-001", "DLV-003")] {
		t.Errorf("DLV-003 depende transitivamente (vía DLV-002) de un at-risk, debería heredar at-risk")
	}
}

// TestResolveAtRiskCycleDoesNotHang es el test obligatorio del plan (§3 Fase
// 3, riesgo "ciclos de dependencias cuelgan el at-risk"): A depende de B y B
// depende de A. Debe resolver sin recursión infinita.
func TestResolveAtRiskCycleDoesNotHang(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	activities := []model.Activity{
		{ID: "ACT-001", Deliverables: []model.Deliverable{
			{ID: "DLV-A", Name: "A", Due: "2026-12-01", Status: "in_progress", DependsOn: strp("DLV-B")},
			{ID: "DLV-B", Name: "B", Due: "2026-12-01", Status: "in_progress", DependsOn: strp("DLV-A")},
		}},
	}
	nodes := buildDeliverableNodes(activities, now)

	done := make(chan map[string]bool, 1)
	go func() { done <- resolveAtRisk(nodes) }()

	select {
	case atRisk := <-done:
		if atRisk[deliverableKey("ACT-001", "DLV-A")] || atRisk[deliverableKey("ACT-001", "DLV-B")] {
			t.Errorf("ni A ni B tienen at-risk propio; el ciclo no debería inventar uno")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resolveAtRisk se colgó con un ciclo A→B→A")
	}
}

func TestMatchesQuadrantFilter(t *testing.T) {
	sc := scoring.ActivityScores{ImpactEffortQuadrant: scoring.QuadrantQuickWins, EisenhowerQuadrant: scoring.EisenhowerDo}
	if !matchesQuadrantFilter(sc, nil) {
		t.Errorf("sin filtro de cuadrante, todo debería matchear")
	}
	if !matchesQuadrantFilter(sc, []string{scoring.QuadrantQuickWins}) {
		t.Errorf("debería matchear por cuadrante Impacto/Esfuerzo")
	}
	if !matchesQuadrantFilter(sc, []string{scoring.EisenhowerDo}) {
		t.Errorf("debería matchear por cuadrante Eisenhower")
	}
	if matchesQuadrantFilter(sc, []string{scoring.EisenhowerDelete}) {
		t.Errorf("no debería matchear un cuadrante que la actividad no tiene")
	}
}
