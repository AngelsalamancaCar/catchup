package svg

import (
	"testing"
	"time"
)

func d(iso string) time.Time {
	t, _ := time.Parse("2006-01-02", iso)
	return t
}

func TestBuildTimelineRowPositionsAndToday(t *testing.T) {
	min, max := d("2026-08-01"), d("2026-09-30")
	now := d("2026-08-15")
	rows := []DeliverableRow{
		{Key: "ACT-001#DLV-001", Name: "A", Due: d("2026-08-01")},
		{Key: "ACT-001#DLV-002", Name: "B", Due: d("2026-09-30")},
	}
	view := BuildTimeline(rows, min, max, now, "timeline de prueba")

	if len(view.Rows) != 2 {
		t.Fatalf("esperaba 2 filas, tengo %d", len(view.Rows))
	}
	if view.Rows[0].MarkerX != view.PlotLeft {
		t.Errorf("primer entregable (min date) debería estar en PlotLeft: %v != %v", view.Rows[0].MarkerX, view.PlotLeft)
	}
	if view.Rows[1].MarkerX != view.PlotRight {
		t.Errorf("último entregable (max date) debería estar en PlotRight: %v != %v", view.Rows[1].MarkerX, view.PlotRight)
	}
	if !view.HasToday {
		t.Errorf("now está dentro del rango, HasToday debería ser true")
	}
	if view.TodayX <= view.PlotLeft || view.TodayX >= view.PlotRight {
		t.Errorf("TodayX fuera del área de ploteo: %v", view.TodayX)
	}
}

func TestBuildTimelineTodayOutOfRange(t *testing.T) {
	min, max := d("2026-08-01"), d("2026-09-30")
	now := d("2026-01-01")
	view := BuildTimeline(nil, min, max, now, "timeline")
	if view.HasToday {
		t.Errorf("now fuera de rango, HasToday debería ser false")
	}
}

func TestBuildTimelineDependencyArrow(t *testing.T) {
	min, max := d("2026-08-01"), d("2026-09-30")
	rows := []DeliverableRow{
		{Key: "ACT-001#DLV-001", Name: "Diseño", Due: d("2026-08-15")},
		{Key: "ACT-001#DLV-002", Name: "Go-live", Due: d("2026-09-30"), DependsOnKey: "ACT-001#DLV-001", AtRisk: true},
	}
	view := BuildTimeline(rows, min, max, d("2026-08-01"), "timeline")
	if len(view.Arrows) != 1 {
		t.Fatalf("esperaba 1 flecha de dependencia, tengo %d", len(view.Arrows))
	}
	if !view.Arrows[0].AtRisk {
		t.Errorf("la flecha debería heredar el at-risk del entregable dependiente")
	}
}

func TestBuildTimelineDanglingDependencyIgnored(t *testing.T) {
	min, max := d("2026-08-01"), d("2026-09-30")
	rows := []DeliverableRow{
		{Key: "ACT-001#DLV-001", Name: "Solo", Due: d("2026-08-15"), DependsOnKey: "ACT-999#NOPE"},
	}
	view := BuildTimeline(rows, min, max, d("2026-08-01"), "timeline")
	if len(view.Arrows) != 0 {
		t.Errorf("dependencia a clave inexistente no debería generar flecha, tengo %d", len(view.Arrows))
	}
}
