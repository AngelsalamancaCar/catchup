package svg

import "testing"

func TestBuildTreemapAreasProportionalToValue(t *testing.T) {
	sections := []SectionAreaInput{
		{Name: "Finanzas", Value: 300, Color: "#111"},
		{Name: "IT", Value: 100, Color: "#222"},
	}
	view := BuildTreemap(sections, "Treemap de prueba")

	if len(view.Rects) != 2 {
		t.Fatalf("esperaba 2 rects, tengo %d", len(view.Rects))
	}
	// Finanzas tiene 3x el valor de IT -> debe tener 3x el ancho (y por
	// tanto 3x el área, ya que la altura es constante).
	wantWidthFinanzas := TreemapWidth * 300.0 / 400.0
	wantWidthIT := TreemapWidth * 100.0 / 400.0
	if !almostEqual(view.Rects[0].Width, wantWidthFinanzas) {
		t.Errorf("ancho Finanzas = %v, esperaba %v", view.Rects[0].Width, wantWidthFinanzas)
	}
	if !almostEqual(view.Rects[1].Width, wantWidthIT) {
		t.Errorf("ancho IT = %v, esperaba %v", view.Rects[1].Width, wantWidthIT)
	}
	if view.Rects[1].X != view.Rects[0].Width {
		t.Errorf("IT debería empezar donde termina Finanzas: X=%v, esperaba %v", view.Rects[1].X, view.Rects[0].Width)
	}
	for _, r := range view.Rects {
		if r.Height != TreemapHeight {
			t.Errorf("altura de %s = %v, esperaba %v (constante)", r.Name, r.Height, TreemapHeight)
		}
	}
}

func TestBuildTreemapZeroTotal(t *testing.T) {
	view := BuildTreemap([]SectionAreaInput{{Name: "Vacía", Value: 0}}, "vacío")
	if len(view.Rects) != 0 {
		t.Errorf("con valor total 0 no debería haber rects, tengo %d", len(view.Rects))
	}
}

func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-6
}
