package svg

import "testing"

func TestXYRangeAndInversion(t *testing.T) {
	if got := X(0); got != PadLeft {
		t.Errorf("X(0) = %v, esperaba %v", got, PadLeft)
	}
	if got := X(100); got != Width-PadRight {
		t.Errorf("X(100) = %v, esperaba %v", got, Width-PadRight)
	}
	// Y está invertido: score alto -> arriba (y pequeño).
	if Y(100) >= Y(0) {
		t.Errorf("Y(100)=%v debería ser menor que Y(0)=%v (arriba)", Y(100), Y(0))
	}
	if got := Y(100); got != PadTop {
		t.Errorf("Y(100) = %v, esperaba %v", got, PadTop)
	}
}

func TestXYClampOutOfRange(t *testing.T) {
	if X(-50) != X(0) {
		t.Errorf("X(-50) debería clampear igual que X(0)")
	}
	if X(150) != X(100) {
		t.Errorf("X(150) debería clampear igual que X(100)")
	}
}

func TestSectionColorStableAndDistinct(t *testing.T) {
	sections := []string{"Operaciones", "Finanzas", "Legal", "IT"}
	c1 := SectionColor(sections, "Finanzas")
	c2 := SectionColor(sections, "Legal")
	if c1 == c2 {
		t.Errorf("secciones distintas deberían tener colores distintos: %s == %s", c1, c2)
	}
	if SectionColor(sections, "Finanzas") != c1 {
		t.Errorf("SectionColor debe ser determinista para la misma sección")
	}
}

func TestSectionColorOrphan(t *testing.T) {
	sections := []string{"Operaciones"}
	if got := SectionColor(sections, "NoExiste"); got != orphanColor {
		t.Errorf("SectionColor huérfana = %s, esperaba %s", got, orphanColor)
	}
}

func TestStrategicFitColorClamps(t *testing.T) {
	if StrategicFitColor(1) != strategicFitScale[0] {
		t.Errorf("StrategicFitColor(1) inesperado")
	}
	if StrategicFitColor(5) != strategicFitScale[4] {
		t.Errorf("StrategicFitColor(5) inesperado")
	}
	if StrategicFitColor(0) != strategicFitScale[0] {
		t.Errorf("StrategicFitColor(0) debería clampear a 1")
	}
	if StrategicFitColor(9) != strategicFitScale[4] {
		t.Errorf("StrategicFitColor(9) debería clampear a 5")
	}
}

func TestPointRadiusKnownAndUnknownBands(t *testing.T) {
	if PointRadius("<5") >= PointRadius(">120") {
		t.Errorf("radio de banda pequeña debería ser menor que la banda grande")
	}
	if PointRadius("banda-invalida") != 6 {
		t.Errorf("PointRadius con banda inválida debería devolver el default 6")
	}
}

func TestBuildMatrixProjectsPoints(t *testing.T) {
	cfg := MatrixConfig{
		XLabel: "Esfuerzo", YLabel: "Impacto",
		ThresholdX: 50, ThresholdY: 50,
		QuadrantTopLeft: "Quick Wins", QuadrantTopRight: "Major Projects",
		QuadrantBottomLeft: "Fill-Ins", QuadrantBottomRight: "Thankless Tasks",
		AriaLabel: "Matriz de prueba",
	}
	points := []PointInput{
		{ID: "ACT-001", X: 20, Y: 80, R: 7, Color: "#000", Label: "A"},
		{ID: "ACT-002", X: 80, Y: 20, R: 9, Color: "#111", Label: "B", Dashed: true, Badge: "i"},
	}
	view := BuildMatrix(cfg, points)

	if len(view.Points) != 2 {
		t.Fatalf("esperaba 2 puntos, tengo %d", len(view.Points))
	}
	if view.Points[0].CX != X(20) || view.Points[0].CY != Y(80) {
		t.Errorf("punto 0 mal proyectado: cx=%v cy=%v", view.Points[0].CX, view.Points[0].CY)
	}
	if !view.Points[1].Dashed || view.Points[1].Badge != "i" {
		t.Errorf("overlay de info no se propagó al punto 1")
	}
	if view.ThresholdXpx != X(50) || view.ThresholdYpx != Y(50) {
		t.Errorf("umbrales mal proyectados")
	}
}
