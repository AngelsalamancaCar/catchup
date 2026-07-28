package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"projectmapper/internal/model"
)

func TestLoadCreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("esperaba que Load escribiera el archivo: %v", err)
	}

	snap := s.Snapshot()
	if len(snap.Sections) != 4 {
		t.Fatalf("esperaba 4 secciones default, tengo %d", len(snap.Sections))
	}
	if snap.Weights.Impact["B1"] != 0.4 {
		t.Fatalf("peso default B1 = %v, esperaba 0.4", snap.Weights.Impact["B1"])
	}
}

func TestRoundtripLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	s1, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := s1.CreateActivity(model.Activity{Name: "Actividad de prueba", Type: "Project"}); err != nil {
		t.Fatalf("CreateActivity: %v", err)
	}
	if err := s1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load reabrir: %v", err)
	}
	snap := s2.Snapshot()
	if len(snap.Activities) != 1 {
		t.Fatalf("esperaba 1 actividad tras reabrir, tengo %d", len(snap.Activities))
	}
	if snap.Activities[0].Name != "Actividad de prueba" {
		t.Fatalf("nombre no sobrevivió el roundtrip: %q", snap.Activities[0].Name)
	}
}

func TestAtomicWriteNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := s.CreateActivity(model.Activity{Name: "A"}); err != nil {
		t.Fatalf("CreateActivity: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("quedó un archivo temporal sin renombrar: %s", e.Name())
		}
	}
}

func TestNextIDSequential(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var ids []string
	for range 3 {
		a, err := s.CreateActivity(model.Activity{Name: "A"})
		if err != nil {
			t.Fatalf("CreateActivity: %v", err)
		}
		ids = append(ids, a.ID)
	}

	want := []string{"ACT-001", "ACT-002", "ACT-003"}
	for i, w := range want {
		if ids[i] != w {
			t.Fatalf("id[%d] = %s, esperaba %s", i, ids[i], w)
		}
	}
}

func TestNextIDAfterDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	a1, _ := s.CreateActivity(model.Activity{Name: "A"})
	a2, _ := s.CreateActivity(model.Activity{Name: "B"})
	if err := s.DeleteActivity(a2.ID); err != nil {
		t.Fatalf("DeleteActivity: %v", err)
	}
	a3, err := s.CreateActivity(model.Activity{Name: "C"})
	if err != nil {
		t.Fatalf("CreateActivity: %v", err)
	}

	if a1.ID != "ACT-001" {
		t.Fatalf("a1.ID = %s", a1.ID)
	}
	// tras borrar ACT-002, el siguiente ID debe seguir siendo secuencial
	// respecto al máximo existente (ACT-001) -> ACT-002 de nuevo.
	if a3.ID != "ACT-002" {
		t.Fatalf("a3.ID = %s, esperaba ACT-002", a3.ID)
	}
}

func TestConcurrentWritesDontCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var wg sync.WaitGroup
	n := 20
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.CreateActivity(model.Activity{Name: "concurrent"}); err != nil {
				t.Errorf("CreateActivity concurrente: %v", err)
			}
		}(i)
	}
	wg.Wait()

	snap := s.Snapshot()
	if len(snap.Activities) != n {
		t.Fatalf("esperaba %d actividades, tengo %d", n, len(snap.Activities))
	}
	seen := map[string]bool{}
	for _, a := range snap.Activities {
		if seen[a.ID] {
			t.Fatalf("ID duplicado tras escrituras concurrentes: %s", a.ID)
		}
		seen[a.ID] = true
	}
}

func TestSetWeightsPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	w := model.Weights{
		Impact: map[string]float64{"B1": 0.25, "B2": 0.25, "B3": 0.25, "B4": 0.25},
		Effort: map[string]float64{"C1": 0.25, "C2": 0.25, "C3": 0.25, "C4": 0.25},
	}
	if err := s.SetWeights(w); err != nil {
		t.Fatalf("SetWeights: %v", err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load reabrir: %v", err)
	}
	if got := s2.Weights().Impact["B1"]; got != 0.25 {
		t.Fatalf("peso B1 tras reabrir = %v, esperaba 0.25", got)
	}
}
