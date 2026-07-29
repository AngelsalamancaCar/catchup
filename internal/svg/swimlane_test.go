package svg

import "testing"

func TestBuildSwimlanesLanesAndOrphan(t *testing.T) {
	sections := []string{"Finanzas", "IT"}
	cards := []ActivityCardInput{
		{ID: "ACT-001", Name: "A", Section: "Finanzas", Involved: []string{"IT"}},
		{ID: "ACT-002", Name: "B", Section: "Legal"}, // sección no configurada -> huérfana
	}
	view := BuildSwimlanes(sections, cards, nil, "org de prueba")

	if len(view.Lanes) != 3 {
		t.Fatalf("esperaba 3 lanes (Finanzas, IT, %s), tengo %d", OrphanLane, len(view.Lanes))
	}
	if view.Lanes[2].Name != OrphanLane {
		t.Errorf("última lane debería ser %q, es %q", OrphanLane, view.Lanes[2].Name)
	}
	if len(view.Cards) != 2 {
		t.Fatalf("esperaba 2 tarjetas, tengo %d", len(view.Cards))
	}
}

func TestBuildSwimlanesConnectorCrossSection(t *testing.T) {
	sections := []string{"Finanzas", "IT"}
	cards := []ActivityCardInput{
		{ID: "ACT-001", Name: "A", Section: "Finanzas", Involved: []string{"IT"}},
	}
	view := BuildSwimlanes(sections, cards, nil, "org")
	if len(view.Connectors) != 1 {
		t.Fatalf("esperaba 1 conector cross-sección, tengo %d", len(view.Connectors))
	}
}

func TestBuildSwimlanesNoSelfConnector(t *testing.T) {
	sections := []string{"Finanzas"}
	cards := []ActivityCardInput{
		{ID: "ACT-001", Name: "A", Section: "Finanzas", Involved: []string{"Finanzas"}},
	}
	view := BuildSwimlanes(sections, cards, nil, "org")
	if len(view.Connectors) != 0 {
		t.Errorf("no debería haber conector de una lane a sí misma, tengo %d", len(view.Connectors))
	}
}

func TestBuildSwimlanesBadgesPropagated(t *testing.T) {
	sections := []string{"Finanzas"}
	badges := map[string][]string{"Finanzas": {"riesgo de coordinación"}}
	view := BuildSwimlanes(sections, nil, badges, "org")
	if len(view.Lanes) != 1 || len(view.Lanes[0].Badges) != 1 {
		t.Fatalf("badge de diagnóstico no se propagó a la lane: %+v", view.Lanes)
	}
	if view.Lanes[0].Badges[0].Text != "riesgo de coordinación" {
		t.Errorf("texto de badge inesperado: %q", view.Lanes[0].Badges[0].Text)
	}
}

func TestBuildSwimlanesWrapsCardsAcrossRows(t *testing.T) {
	sections := []string{"Operaciones"}
	perRow := cardsPerRow()
	var cards []ActivityCardInput
	for i := 0; i < perRow+1; i++ {
		cards = append(cards, ActivityCardInput{ID: string(rune('A' + i)), Section: "Operaciones"})
	}
	view := BuildSwimlanes(sections, cards, nil, "org")
	// La tarjeta perRow (índice 0-based) debería caer en la segunda fila,
	// es decir, más abajo que la primera tarjeta de la lane.
	if view.Cards[perRow].Y <= view.Cards[0].Y {
		t.Errorf("la tarjeta %d debería envolver a una fila nueva (Y mayor)", perRow)
	}
}
