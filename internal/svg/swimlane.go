package svg

import "fmt"

// Constantes de layout del swimlane organizacional.
const (
	OrgWidth       = 760.0
	laneLabelWidth = 130.0
	lanePad        = 10.0
	laneHeaderBase = 18.0 // alto de la etiqueta de sección
	badgeLineH     = 12.0 // alto de cada línea de anotación de diagnóstico
	cardWidth      = 150.0
	cardHeight     = 34.0
	cardGapX       = 10.0
	cardGapY       = 8.0
	minLaneHeight  = 60.0

	// OrphanLane es el nombre de la lane donde caen las actividades sin
	// sección asignada — regla "huérfana" de §2.4 del plan.
	OrphanLane = "Sin asignar"
)

// ActivityCardInput es una actividad a ubicar en su lane organizacional.
type ActivityCardInput struct {
	ID, Name, Section, Quadrant, DetailURL string
	Involved                               []string
}

// RenderedCard es una tarjeta de actividad ya posicionada en píxeles.
type RenderedCard struct {
	ID, Name, Quadrant, DetailURL string
	X, Y, Width, Height           float64
}

// Connector es un path bezier ya calculado entre una tarjeta y la lane de
// una sección involucrada (A5), listo para interpolar en el template.
type Connector struct {
	PathD string
}

// BadgeLine es una anotación de diagnóstico de una lane, ya posicionada.
type BadgeLine struct {
	Text string
	Y    float64
}

// LaneHeader es la cabecera de una lane con sus anotaciones de diagnóstico
// automáticas (§2.4: riesgo de coordinación, candidata a revisión).
type LaneHeader struct {
	Name      string
	Y, Height float64
	LabelY    float64
	Badges    []BadgeLine
}

// SwimlaneView es la vista completamente precomputada de swimlanes que
// consume swimlane_svg.svg.tmpl (no hace ninguna aritmética).
type SwimlaneView struct {
	Width, Height float64
	Lanes         []LaneHeader
	Cards         []RenderedCard
	Connectors    []Connector
	AriaLabel     string
}

func cardsPerRow() int {
	plotWidth := OrgWidth - laneLabelWidth - lanePad
	n := int(plotWidth / (cardWidth + cardGapX))
	if n < 1 {
		return 1
	}
	return n
}

// BuildSwimlanes ordena las actividades en lanes por sección (con
// OrphanLane al final si hay huérfanas), calcula la altura de cada lane
// según sus anotaciones y cuántas tarjetas caben por fila, y arma los
// conectores curvos hacia las lanes involucradas (A5). Helper de render
// puro, sin I/O.
func BuildSwimlanes(sections []string, cards []ActivityCardInput, badges map[string][]string, ariaLabel string) SwimlaneView {
	laneNames := append([]string{}, sections...)
	hasOrphan := false
	for _, c := range cards {
		if indexOf(sections, c.Section) < 0 {
			hasOrphan = true
		}
	}
	if hasOrphan {
		laneNames = append(laneNames, OrphanLane)
	}

	byLane := map[string][]ActivityCardInput{}
	for _, c := range cards {
		lane := c.Section
		if indexOf(sections, lane) < 0 {
			lane = OrphanLane
		}
		byLane[lane] = append(byLane[lane], c)
	}

	perRow := cardsPerRow()
	view := SwimlaneView{Width: OrgWidth, AriaLabel: ariaLabel}
	y := lanePad
	laneCenterY := map[string]float64{}

	for _, name := range laneNames {
		items := byLane[name]
		badgeList := badges[name]
		headerH := laneHeaderBase + float64(len(badgeList))*badgeLineH + 6

		rows := 1
		if len(items) > 0 {
			rows = (len(items) + perRow - 1) / perRow
		}
		height := headerH + float64(rows)*(cardHeight+cardGapY) + lanePad
		if height < minLaneHeight {
			height = minLaneHeight
		}
		laneCenterY[name] = y + height/2

		labelY := y + 14
		var badgeLines []BadgeLine
		for i, b := range badgeList {
			badgeLines = append(badgeLines, BadgeLine{Text: b, Y: labelY + float64(i+1)*badgeLineH})
		}
		view.Lanes = append(view.Lanes, LaneHeader{Name: name, Y: y, Height: height, LabelY: labelY, Badges: badgeLines})

		for i, c := range items {
			row := i / perRow
			col := i % perRow
			view.Cards = append(view.Cards, RenderedCard{
				ID: c.ID, Name: c.Name, Quadrant: c.Quadrant, DetailURL: c.DetailURL,
				X:      laneLabelWidth + lanePad + float64(col)*(cardWidth+cardGapX),
				Y:      y + headerH + float64(row)*(cardHeight+cardGapY),
				Width:  cardWidth,
				Height: cardHeight,
			})
		}

		y += height
	}
	view.Height = y + lanePad

	cardCenter := make(map[string][2]float64, len(view.Cards))
	for _, rc := range view.Cards {
		cardCenter[rc.ID] = [2]float64{rc.X + rc.Width, rc.Y + rc.Height/2}
	}
	for _, c := range cards {
		from, ok := cardCenter[c.ID]
		if !ok {
			continue
		}
		for _, inv := range c.Involved {
			if inv == c.Section || indexOf(sections, inv) < 0 {
				continue
			}
			ty, ok := laneCenterY[inv]
			if !ok {
				continue
			}
			view.Connectors = append(view.Connectors, Connector{PathD: bezierPath(from[0], from[1], laneLabelWidth, ty)})
		}
	}

	return view
}

func bezierPath(x1, y1, x2, y2 float64) string {
	midX := (x1 + x2) / 2
	return fmt.Sprintf("M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f", x1, y1, midX, y1, midX, y2, x2, y2)
}
