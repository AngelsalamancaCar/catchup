package server

import (
	"slices"
	"testing"
	"time"

	"projectmapper/internal/model"
)

func sampleSnapshot() model.Store {
	d1 := "2026-08-01"
	return model.Store{
		Sections:   []string{"Finanzas", "IT"},
		Weights:    model.DefaultWeights(),
		Thresholds: model.DefaultThresholds(),
		Activities: []model.Activity{
			{
				ID: "ACT-001", Name: "Migración ERP", Type: "Project", Owner: "Marta", Section: "Finanzas",
				Involved: []string{"IT"},
				Answers: model.Answers{
					B1: 5, B2: 4, B3: 5, B4: 4,
					C1: ">120", C2: 4, C3: 5, C4: "blocking",
					D1: &d1, D2: "escalation", D3: "shared", D4: "no",
					F1: 3, F2: []string{"C1"}, F3: "falta alcance",
				},
				Deliverables: []model.Deliverable{
					{ID: "DLV-001", Name: "Diseño", Due: "2026-08-15", Status: "in_progress"},
				},
			},
			{
				ID: "ACT-002", Name: "Reporte semanal", Type: "Workstream", Owner: "Carlos", Section: "IT",
				Answers: model.Answers{
					B1: 3, B2: 3, B3: 2, B4: 2,
					C1: "5-20", C2: 1, C3: 2, C4: "none",
					D2: "friction", D3: "mine", D4: "yes",
					F1: 5,
				},
			},
		},
	}
}

func TestBuildPortfolioWorkbookHasAllSheets(t *testing.T) {
	f, err := buildPortfolioWorkbook(sampleSnapshot(), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildPortfolioWorkbook: %v", err)
	}
	defer f.Close()

	want := []string{"Actividades", "Deliverables", "Matriz IE", "Eisenhower", "Resumen"}
	got := f.GetSheetList()
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("falta la hoja %q, hojas presentes: %v", w, got)
		}
	}
}

func TestBuildPortfolioWorkbookActivitiesDataMatchesScoring(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	snap := sampleSnapshot()
	f, err := buildPortfolioWorkbook(snap, now)
	if err != nil {
		t.Fatalf("buildPortfolioWorkbook: %v", err)
	}
	defer f.Close()

	name, _ := f.GetCellValue("Actividades", "B2")
	if name != "Migración ERP" {
		t.Errorf("B2 = %q, esperaba el nombre de ACT-001", name)
	}
	quadrant, _ := f.GetCellValue("Actividades", "AC2")
	if quadrant != "Major Projects" {
		t.Errorf("cuadrante I/E de ACT-001 = %q, esperaba Major Projects (impacto y esfuerzo altos)", quadrant)
	}
}

func TestBuildPortfolioWorkbookEmptyActivitiesNoChart(t *testing.T) {
	snap := model.Store{Sections: []string{"Finanzas"}, Weights: model.DefaultWeights(), Thresholds: model.DefaultThresholds()}
	f, err := buildPortfolioWorkbook(snap, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildPortfolioWorkbook con store vacío no debería fallar: %v", err)
	}
	defer f.Close()
}
