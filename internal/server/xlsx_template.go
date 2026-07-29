package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"projectmapper/internal/svg"
)

// Nombres de las hojas del template de carga masiva. La hoja Actividades y la
// hoja Entregables son las dos únicas que lee el import (xlsx_import.go); las
// demás son ayuda para quien llena el archivo.
const (
	sheetInstructions = "Instrucciones"
	sheetActivities   = "Actividades"
	sheetDeliverables = "Entregables"
	sheetGantt        = "Gantt"
	sheetLists        = "Listas"
)

// Layout de las hojas de captura: banda de grupos, encabezados y datos. El
// import no asume estas filas (busca la fila de encabezados), pero sí las usan
// las fórmulas y validaciones del template.
const (
	bandRow      = 1
	headerRow    = 2
	firstDataRow = 3
)

const (
	// templateActivityRows / templateDeliverableRows son las filas que llevan
	// dropdown y formato en el template (basta para una carga inicial; el
	// import no tiene tope de filas).
	templateActivityRows    = 200
	templateDeliverableRows = 400
	// ganttRows son las filas de Entregables cableadas al gráfico de Gantt.
	ganttRows = 40

	// exampleRowPrefix marca las filas de ejemplo que trae el template. El
	// import las omite, para que el archivo se pueda subir sin borrarlas.
	exampleRowPrefix = "EJEMPLO"
)

// Colores del template. Los de grupo salen de la paleta de secciones de la app
// (internal/svg), para que el Excel se lea como parte de Catchup y no como una
// hoja genérica.
const (
	inkColor    = "2C2C34" // texto principal
	mutedColor  = "6E7180" // texto secundario
	zebraColor  = "F6F5FB" // fila par de la zona de captura
	missingFill = "FFF3D6" // falta un dato obligatorio
	missingInk  = "8A5A00"
	okFill      = "E7F5EC"
	okInk       = "1F6B3B"
	borderColor = "DCDAE8"
)

func groupColor(group string) string {
	switch group {
	case groupIdent:
		return svg.SectionPalette[4] // #423a6a
	case groupImpact:
		return svg.SectionPalette[2] // #796cbf
	case groupEffort:
		return svg.SectionPalette[0] // #b5abfc
	case groupUrgency:
		return svg.SectionPalette[3] // #75798c
	case groupInfo:
		return svg.SectionPalette[1] // #b2b6ca
	default:
		return svg.SectionPalette[2]
	}
}

func hex(color string) string { return strings.TrimPrefix(color, "#") }

// xw acumula el primer error de una secuencia de escrituras en excelize: el
// template son cientos de llamadas y comprobar cada una por separado enterraría
// la estructura del documento en `if err != nil`.
type xw struct {
	f   *excelize.File
	err error
}

func (x *xw) do(err error) {
	if x.err == nil && err != nil {
		x.err = err
	}
}

func (x *xw) cell(sheet, cell string, v any)          { x.do(x.f.SetCellValue(sheet, cell, v)) }
func (x *xw) formula(sheet, cell, f string)           { x.do(x.f.SetCellFormula(sheet, cell, f)) }
func (x *xw) style(sheet, tl, br string, styleID int) { x.do(x.f.SetCellStyle(sheet, tl, br, styleID)) }
func (x *xw) width(sheet, c1, c2 string, w float64)   { x.do(x.f.SetColWidth(sheet, c1, c2, w)) }
func (x *xw) merge(sheet, tl, br string)              { x.do(x.f.MergeCell(sheet, tl, br)) }
func (x *xw) rowHeight(sheet string, row int, h float64) {
	x.do(x.f.SetRowHeight(sheet, row, h))
}

func (x *xw) newStyle(s *excelize.Style) int {
	id, err := x.f.NewStyle(s)
	x.do(err)
	return id
}

func (x *xw) newCondStyle(s *excelize.Style) int {
	id, err := x.f.NewConditionalStyle(s)
	x.do(err)
	return id
}

func (x *xw) newSheet(name string) {
	if _, err := x.f.NewSheet(name); err != nil {
		x.do(err)
	}
}

// hideGridLines deja la hoja sin cuadrícula: las hojas de lectura
// (Instrucciones, Gantt) se ven como documento y no como planilla.
func (x *xw) hideGridLines(sheet string) {
	x.do(x.f.SetSheetView(sheet, -1, &excelize.ViewOptions{ShowGridLines: new(false)}))
}

func (x *xw) tabColor(sheet, color string) {
	// Excel espera el color de pestaña en ARGB.
	rgb := "FF" + strings.ToUpper(hex(color))
	x.do(x.f.SetSheetProps(sheet, &excelize.SheetPropsOptions{TabColorRGB: &rgb}))
}

// templateStyles son los estilos compartidos por las hojas del template.
type templateStyles struct {
	title    int
	subtitle int
	lead     int
	body     int
	label    int
	labelTop int
	date     int
	example  int
	exDate   int
	wrap     int
	status   int
	helper   int

	// Por grupo de columnas: banda superior y encabezado.
	band   map[string]int
	header map[string]int

	// Formatos condicionales de la zona de captura.
	cfZebra   int
	cfMissing int
	cfOK      int
	cfWarn    int
}

func newTemplateStyles(x *xw, groups []string) templateStyles {
	thinBorder := []excelize.Border{
		{Type: "left", Color: borderColor, Style: 1},
		{Type: "right", Color: borderColor, Style: 1},
		{Type: "top", Color: borderColor, Style: 1},
		{Type: "bottom", Color: borderColor, Style: 1},
	}

	st := templateStyles{
		title: x.newStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 20, Color: hex(svg.SectionPalette[4])},
		}),
		subtitle: x.newStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 13, Color: hex(svg.SectionPalette[2])},
		}),
		lead: x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Size: 11, Color: mutedColor},
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		}),
		body: x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Size: 11, Color: inkColor},
			Alignment: &excelize.Alignment{Vertical: "top"},
		}),
		label: x.newStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: inkColor}}),
		labelTop: x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: inkColor},
			Alignment: &excelize.Alignment{Vertical: "top"},
		}),
		date: x.newStyle(&excelize.Style{
			CustomNumFmt: new("yyyy-mm-dd"),
			Alignment:    &excelize.Alignment{Horizontal: "center"},
		}),
		example: x.newStyle(&excelize.Style{Font: &excelize.Font{Italic: true, Color: mutedColor}}),
		exDate: x.newStyle(&excelize.Style{
			Font:         &excelize.Font{Italic: true, Color: mutedColor},
			CustomNumFmt: new("yyyy-mm-dd"),
			Alignment:    &excelize.Alignment{Horizontal: "center"},
		}),
		wrap: x.newStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
			Font:      &excelize.Font{Size: 10.5, Color: inkColor},
			Border:    thinBorder,
		}),
		status: x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 10.5, Color: mutedColor},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		}),
		helper: x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Size: 10.5, Color: mutedColor},
			Alignment: &excelize.Alignment{Horizontal: "center"},
		}),
		band:    map[string]int{},
		header:  map[string]int{},
		cfZebra: x.newCondStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{zebraColor}}}),
		cfMissing: x.newCondStyle(&excelize.Style{
			Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{missingFill}},
			Font: &excelize.Font{Color: missingInk},
		}),
		cfOK: x.newCondStyle(&excelize.Style{
			Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{okFill}},
			Font: &excelize.Font{Bold: true, Color: okInk},
		}),
		cfWarn: x.newCondStyle(&excelize.Style{
			Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{missingFill}},
			Font: &excelize.Font{Bold: true, Color: missingInk},
		}),
	}

	for _, g := range groups {
		color := groupColor(g)
		st.band[g] = x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 11, Color: hex(svg.TextOnColor(color))},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{hex(color)}},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		})
		// El encabezado va en blanco con subrayado del color del grupo: se lee
		// mejor que texto sobre fondo saturado y mantiene la pista de color.
		st.header[g] = x.newStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 10.5, Color: inkColor},
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "bottom", Horizontal: "left"},
			Border: []excelize.Border{
				{Type: "bottom", Color: hex(color), Style: 5},
				{Type: "right", Color: borderColor, Style: 1},
			},
		})
	}
	return st
}

func colLetter(index int) string {
	name, err := excelize.ColumnNumberToName(index)
	if err != nil {
		return "A"
	}
	return name
}

// letterOf devuelve la letra de columna del encabezado dado dentro de cols.
// Se usa para que las fórmulas del Gantt no hardcodeen posiciones de la hoja
// Entregables: si se reordenan las columnas, las fórmulas siguen apuntando bien.
func letterOf(cols []xlsxColumn, header string) string {
	for i, c := range cols {
		if c.Header == header {
			return colLetter(i + 1)
		}
	}
	return "A"
}

// columnGroups devuelve los grupos en el orden en que aparecen.
func columnGroups(colSets ...[]xlsxColumn) []string {
	var out []string
	seen := map[string]bool{}
	for _, cols := range colSets {
		for _, c := range cols {
			if c.Group != "" && !seen[c.Group] {
				seen[c.Group] = true
				out = append(out, c.Group)
			}
		}
	}
	return out
}

// helperColumn es una columna calculada que el template agrega a la derecha de
// los datos (semáforo de captura, duración). El import las ignora porque mapea
// por nombre de encabezado.
type helperColumn struct {
	Header  string
	Width   float64
	Comment string
	Formula func(row int) string
	Rules   []excelize.ConditionalFormatOptions
}

// buildImportTemplateWorkbook arma el .xlsx de carga masiva: instrucciones,
// hojas de captura con desplegables y semáforo de filas incompletas, y una hoja
// Gantt cuyas fórmulas y gráfico de barras apiladas se alimentan de Entregables.
func buildImportTemplateWorkbook(sections []string, now time.Time) (*excelize.File, error) {
	if len(sections) == 0 {
		sections = []string{"Sin sección"}
	}
	x := &xw{f: excelize.NewFile()}
	x.do(x.f.SetSheetName("Sheet1", sheetInstructions))
	for _, name := range []string{sheetActivities, sheetDeliverables, sheetGantt, sheetLists} {
		x.newSheet(name)
	}

	actCols, dlvCols := activityColumns(), deliverableColumns()
	st := newTemplateStyles(x, columnGroups(actCols, dlvCols))
	lists := x.writeListsSheet(actCols, dlvCols, sections, st)

	x.writeGridSheet(sheetActivities, actCols, templateActivityRows, lists, nil, st,
		activityHelperColumns(actCols, st), exampleActivityRow(sections, now))
	x.writeGridSheet(sheetDeliverables, dlvCols, templateDeliverableRows, lists,
		deliverableNameLists(actCols, dlvCols), st,
		deliverableHelperColumns(dlvCols, st), nil)
	for i, row := range exampleDeliverableRows(now) {
		x.writeExampleRow(sheetDeliverables, dlvCols, firstDataRow+i, row, st)
	}
	x.writeGanttSheet(dlvCols, st)
	x.writeInstructionsSheet(actCols, dlvCols, sections, st)

	for sheet, color := range map[string]string{
		sheetInstructions: svg.SectionPalette[4],
		sheetActivities:   svg.SectionPalette[2],
		sheetDeliverables: svg.SectionPalette[0],
		sheetGantt:        svg.SectionPalette[3],
		sheetLists:        svg.SectionPalette[1],
	} {
		x.tabColor(sheet, color)
	}

	// La hoja Listas solo alimenta los dropdowns; se oculta para no invitar a
	// editarla por error (las validaciones la siguen leyendo igual).
	x.do(x.f.SetSheetVisible(sheetLists, false))
	// Las fórmulas del Gantt y de los semáforos se escriben sin valor cacheado:
	// hay que pedirle a Excel/LibreOffice que recalcule al abrir el archivo.
	x.do(x.f.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: new(true)}))
	x.f.SetActiveSheet(0)

	if x.err != nil {
		x.f.Close()
		return nil, x.err
	}
	return x.f, nil
}

// writeListsSheet vuelca los valores válidos de cada columna en la hoja Listas
// y devuelve, por nombre de lista, la referencia de rango que usan los
// dropdowns (las listas por rango evitan el límite de 255 caracteres y los
// problemas de escapado de "<5" / ">120" en listas inline).
func (x *xw) writeListsSheet(actCols, dlvCols []xlsxColumn, sections []string, st templateStyles) map[string]string {
	refs := map[string]string{}
	col := 1
	for _, c := range append(append([]xlsxColumn{}, actCols...), dlvCols...) {
		name := c.listName()
		if name == "" || refs[name] != "" {
			continue
		}
		values := c.listValues(sections)
		if len(values) == 0 {
			continue
		}
		letter := colLetter(col)
		x.cell(sheetLists, letter+"1", name)
		x.style(sheetLists, letter+"1", letter+"1", st.label)
		for i, v := range values {
			x.cell(sheetLists, fmt.Sprintf("%s%d", letter, i+2), v)
		}
		x.width(sheetLists, letter, letter, 42)
		refs[name] = fmt.Sprintf("%s!$%s$2:$%s$%d", sheetLists, letter, letter, len(values)+1)
		col++
	}
	return refs
}

// writeGridSheet escribe una hoja de captura: banda de grupos, encabezados con
// comentario de ayuda, desplegables, franjas alternas, resaltado de obligatorios
// vacíos y columnas calculadas de apoyo.
func (x *xw) writeGridSheet(sheet string, cols []xlsxColumn, rows int, lists map[string]string, dynamic map[string]dynamicList, st templateStyles, helpers []helperColumn, example []any) {
	lastRow := firstDataRow + rows - 1

	x.writeGroupBand(sheet, cols, st)

	for i, c := range cols {
		letter := colLetter(i + 1)
		header := c.Header
		if c.Required {
			header += " *"
		}
		x.cell(sheet, fmt.Sprintf("%s%d", letter, headerRow), header)
		x.style(sheet, fmt.Sprintf("%s%d", letter, headerRow), fmt.Sprintf("%s%d", letter, headerRow), st.header[c.Group])
		x.width(sheet, letter, letter, c.Width)

		x.do(x.f.AddComment(sheet, excelize.Comment{
			Cell:   fmt.Sprintf("%s%d", letter, headerRow),
			Author: "Catchup",
			Text:   headerHelp(c),
			Width:  300, Height: 140,
		}))

		dataRange := fmt.Sprintf("%s%d:%s%d", letter, firstDataRow, letter, lastRow)
		if c.Kind == colDate {
			x.style(sheet, fmt.Sprintf("%s%d", letter, firstDataRow), fmt.Sprintf("%s%d", letter, lastRow), st.date)
		}

		// Una sola validación por columna: dos sobre el mismo rango se pisan
		// entre sí en Excel. Prioridad: lista viva (nombres de otra hoja) →
		// lista fija de la hoja Listas → solo mensaje de ayuda.
		switch d, dyn := dynamic[c.Header]; {
		case dyn:
			dv := excelize.NewDataValidation(true)
			dv.Sqref = dataRange
			dv.SetSqrefDropList(d.ref)
			dv.SetInput(c.Question, d.prompt)
			if d.warn != "" {
				dv.SetError(excelize.DataValidationErrorStyleWarning, c.Header, d.warn)
			}
			x.do(x.f.AddDataValidation(sheet, dv))
		case lists[c.listName()] != "":
			dv := excelize.NewDataValidation(true)
			dv.Sqref = dataRange
			dv.SetSqrefDropList(lists[c.listName()])
			dv.SetError(excelize.DataValidationErrorStyleStop, c.Header,
				"Elige una opción de la lista desplegable.\n\n"+c.validValuesText())
			dv.SetInput(c.Question, c.validValuesText())
			x.do(x.f.AddDataValidation(sheet, dv))
		case c.Kind != colDate:
			// Sin lista no hay validación, pero sí la ayuda al seleccionar la celda.
			dv := excelize.NewDataValidation(true)
			dv.Sqref = dataRange
			dv.SetInput(c.Question, c.validValuesText())
			x.do(x.f.AddDataValidation(sheet, dv))
		}

		// Franja alterna en toda la zona de captura y, en las columnas
		// obligatorias, resaltado ámbar si la fila tiene nombre pero le falta
		// este dato: el error se ve al llenar, no al subir el archivo.
		rules := []excelize.ConditionalFormatOptions{
			{Type: "formula", Criteria: "=MOD(ROW(),2)=0", Format: &st.cfZebra},
		}
		if c.Required {
			rules = append([]excelize.ConditionalFormatOptions{{
				Type:     "formula",
				Criteria: fmt.Sprintf(`=AND($A%d<>"",%s%d="")`, firstDataRow, letter, firstDataRow),
				Format:   &st.cfMissing,
			}}, rules...)
		}
		x.do(x.f.SetConditionalFormat(sheet, dataRange, rules))
	}

	for i, h := range helpers {
		letter := colLetter(len(cols) + 1 + i)
		x.cell(sheet, fmt.Sprintf("%s%d", letter, bandRow), "Automático")
		x.style(sheet, fmt.Sprintf("%s%d", letter, bandRow), fmt.Sprintf("%s%d", letter, bandRow), st.status)
		x.cell(sheet, fmt.Sprintf("%s%d", letter, headerRow), h.Header)
		x.style(sheet, fmt.Sprintf("%s%d", letter, headerRow), fmt.Sprintf("%s%d", letter, headerRow), st.header[groupIdent])
		x.width(sheet, letter, letter, h.Width)
		if h.Comment != "" {
			x.do(x.f.AddComment(sheet, excelize.Comment{
				Cell: fmt.Sprintf("%s%d", letter, headerRow), Author: "Catchup", Text: h.Comment,
				Width: 280, Height: 100,
			}))
		}
		for row := firstDataRow; row <= lastRow; row++ {
			x.formula(sheet, fmt.Sprintf("%s%d", letter, row), h.Formula(row))
		}
		cell := fmt.Sprintf("%s%d", letter, firstDataRow)
		x.style(sheet, cell, fmt.Sprintf("%s%d", letter, lastRow), st.helper)
		if len(h.Rules) > 0 {
			x.do(x.f.SetConditionalFormat(sheet, fmt.Sprintf("%s%d:%s%d", letter, firstDataRow, letter, lastRow), h.Rules))
		}
	}

	x.rowHeight(sheet, bandRow, 20)
	x.rowHeight(sheet, headerRow, 34)
	// Congelar encabezados y la primera columna: al desplazarse a la derecha se
	// sigue viendo de qué actividad es la fila.
	x.do(x.f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 1, YSplit: headerRow, TopLeftCell: fmt.Sprintf("B%d", firstDataRow),
		ActivePane: "bottomRight",
		Selection: []excelize.Selection{
			{SQRef: fmt.Sprintf("A%d", firstDataRow), ActiveCell: fmt.Sprintf("A%d", firstDataRow), Pane: "bottomRight"},
		},
	}))

	if len(example) > 0 {
		x.writeExampleRow(sheet, cols, firstDataRow, example, st)
	}
}

// writeGroupBand pinta la fila 1 con un bloque de color por grupo de columnas
// (A Identificación, B Impacto, …), que es lo que da la lectura de "formulario"
// a la hoja.
func (x *xw) writeGroupBand(sheet string, cols []xlsxColumn, st templateStyles) {
	start := 0
	for i := 0; i <= len(cols); i++ {
		if i < len(cols) && cols[i].Group == cols[start].Group {
			continue
		}
		group := cols[start].Group
		from, to := colLetter(start+1), colLetter(i)
		x.cell(sheet, fmt.Sprintf("%s%d", from, bandRow), group)
		if from != to {
			x.merge(sheet, fmt.Sprintf("%s%d", from, bandRow), fmt.Sprintf("%s%d", to, bandRow))
		}
		x.style(sheet, fmt.Sprintf("%s%d", from, bandRow), fmt.Sprintf("%s%d", to, bandRow), st.band[group])
		start = i
	}
}

// requiredRanges agrupa las columnas obligatorias en rangos contiguos, para que
// el semáforo de fila pueda contar celdas vacías con pocos COUNTBLANK.
func requiredRanges(cols []xlsxColumn) [][2]int {
	var out [][2]int
	for i, c := range cols {
		if !c.Required {
			continue
		}
		if n := len(out); n > 0 && out[n-1][1] == i-1 {
			out[n-1][1] = i
			continue
		}
		out = append(out, [2]int{i, i})
	}
	return out
}

// rowStatusFormula arma la fórmula del semáforo: cuenta las celdas obligatorias
// vacías de la fila y devuelve "Completa" / "Faltan datos" (o vacío si la fila
// no se ha empezado). Se mantiene deliberadamente simple —sin concatenar el
// número de faltantes— porque esa variante confunde al motor de fórmulas de
// LibreOffice/excelize al anidarla dentro de otro IF.
func rowStatusFormula(cols []xlsxColumn, keyCol string, row int) string {
	var parts []string
	for _, r := range requiredRanges(cols) {
		parts = append(parts, fmt.Sprintf("COUNTBLANK(%s%d:%s%d)",
			colLetter(r[0]+1), row, colLetter(r[1]+1), row))
	}
	return fmt.Sprintf(`IF($%s%d="","",IF(%s=0,"Completa","Faltan datos"))`,
		keyCol, row, strings.Join(parts, "+"))
}

func statusRules(st templateStyles) []excelize.ConditionalFormatOptions {
	// "containing" es el nombre que excelize le da al criterio containsText.
	return []excelize.ConditionalFormatOptions{
		{Type: "text", Criteria: "containing", Value: "Completa", Format: &st.cfOK},
		{Type: "text", Criteria: "containing", Value: "Faltan", Format: &st.cfWarn},
	}
}

func activityHelperColumns(cols []xlsxColumn, st templateStyles) []helperColumn {
	return []helperColumn{{
		Header:  "¿Fila completa?",
		Width:   18,
		Comment: "Columna calculada: avisa si la fila tiene datos obligatorios vacíos. Catchup la ignora al importar.",
		Formula: func(row int) string { return rowStatusFormula(cols, "A", row) },
		Rules:   statusRules(st),
	}}
}

func deliverableHelperColumns(cols []xlsxColumn, st templateStyles) []helperColumn {
	startCol := letterOf(cols, "Inicio")
	endCol := letterOf(cols, "Fin")
	return []helperColumn{
		{
			Header:  "Duración (días)",
			Width:   16,
			Comment: "Columna calculada: Fin − Inicio, la misma barra que dibuja la hoja Gantt.",
			Formula: func(row int) string {
				return fmt.Sprintf(`IF(OR($A%[1]d="",$%[3]s%[1]d=""),"",MAX(1,$%[3]s%[1]d-IF($%[2]s%[1]d="",$%[3]s%[1]d,$%[2]s%[1]d)))`,
					row, startCol, endCol)
			},
		},
		{
			Header:  "¿Fila completa?",
			Width:   18,
			Comment: "Columna calculada: avisa si la fila tiene datos obligatorios vacíos. Catchup la ignora al importar.",
			Formula: func(row int) string { return rowStatusFormula(cols, "A", row) },
			Rules:   statusRules(st),
		},
	}
}

// dynamicList es un desplegable cuya fuente es un rango vivo de otra hoja (los
// nombres que el usuario va escribiendo) y no una lista fija de la hoja Listas.
type dynamicList struct {
	ref    string
	prompt string
	warn   string // mensaje al escribir algo fuera de la lista; vacío = no avisar
}

// deliverableNameLists arma los desplegables por nombre de la hoja Entregables:
// la actividad dueña se elige de lo capturado en la hoja Actividades, y la
// dependencia de los entregables ya escritos. Es lo que evita el error más común
// de la carga (un nombre de actividad que no existe).
func deliverableNameLists(actCols, dlvCols []xlsxColumn) map[string]dynamicList {
	rangeOf := func(sheet, col string, rows int) string {
		return fmt.Sprintf("%s!$%s$%d:$%s$%d", sheet, col, firstDataRow, col, firstDataRow+rows-1)
	}
	return map[string]dynamicList{
		"Actividad": {
			ref:    rangeOf(sheetActivities, letterOf(actCols, "Nombre"), templateActivityRows),
			prompt: "Elige una de las actividades que capturaste en la hoja Actividades.",
			warn:   "Ese nombre no está en la hoja Actividades. Si lo dejas así, la carga fallará con un error en esta fila.",
		},
		"Depende de": {
			ref:    rangeOf(sheetDeliverables, letterOf(dlvCols, "Entregable"), templateDeliverableRows),
			prompt: "Opcional: elige otro entregable de la misma actividad.",
		},
	}
}

// writeExampleRow escribe una fila de ejemplo en gris cursiva. El import omite
// estas filas por el prefijo EJEMPLO de la primera celda, así que el archivo se
// puede subir tal cual sin ensuciar el portafolio.
func (x *xw) writeExampleRow(sheet string, cols []xlsxColumn, row int, values []any, st templateStyles) {
	for i, v := range values {
		if i >= len(cols) {
			break
		}
		letter := colLetter(i + 1)
		cell := fmt.Sprintf("%s%d", letter, row)
		x.cell(sheet, cell, v)
		style := st.example
		if cols[i].Kind == colDate {
			style = st.exDate
		}
		x.style(sheet, cell, cell, style)
	}
}

// headerHelp es el texto del comentario que Excel muestra sobre el encabezado.
func headerHelp(c xlsxColumn) string {
	var b strings.Builder
	b.WriteString(c.Question)
	if c.Required {
		b.WriteString(" (obligatorio)")
	}
	b.WriteString("\n\nValores: ")
	b.WriteString(c.validValuesText())
	if c.Help != "" {
		b.WriteString("\n\n")
		b.WriteString(c.Help)
	}
	return b.String()
}

// exampleDate devuelve una fecha (sin hora) como time.Time: escrita así, la
// celda queda como fecha real de Excel y no como texto, que es lo que necesitan
// las restas de días del Gantt.
func exampleDate(now time.Time, months int) time.Time {
	d := now.AddDate(0, months, 0)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// scaleExample es la opción de desplegable correspondiente al valor v de una
// pregunta 1-5, para que la fila de ejemplo se vea igual que una fila llenada
// con el desplegable.
func scaleExample(question string, v int) string {
	opts := anchors(question)
	if v < 1 || v > len(opts) {
		return fmt.Sprint(v)
	}
	return scaleOption(opts[v-1])
}

// exampleActivityName se mantiene corto porque también es la etiqueta que el
// Gantt muestra en su eje: un nombre largo se recorta en el gráfico.
const exampleActivityName = exampleRowPrefix + " Migración ERP"

func exampleActivityRow(sections []string, now time.Time) []any {
	involved := ""
	if len(sections) > 1 {
		involved = sections[1]
	}
	return []any{
		exampleActivityName, typeCoded.label("Project"), "Marta", sections[0], involved,
		"Fila de ejemplo: puedes borrarla o dejarla, Catchup la ignora al importar.",
		scaleExample("b1", 5), scaleExample("b2", 4), scaleExample("b3", 5), scaleExample("b4", 4),
		c1Coded.label("60-120"), scaleExample("c2", 4), scaleExample("c3", 4), c4Coded.label("some"),
		exampleDate(now, 3), d2Coded.label("escalation"), d3Coded.label("shared"), d4Coded.label("no"),
		scaleExample("f1", 3), "B3;C1", "Falta el alcance definitivo del módulo de tesorería (lo tiene Finanzas).",
	}
}

func exampleDeliverableRows(now time.Time) [][]any {
	d := func(months int) time.Time { return exampleDate(now, months) }
	diseño := exampleRowPrefix + " Diseño"
	datos := exampleRowPrefix + " Datos"
	return [][]any{
		{exampleActivityName, diseño, d(0), d(1), statusCoded.label("in_progress"), ""},
		{exampleActivityName, datos, d(1), d(2), statusCoded.label("not_started"), diseño},
		{exampleActivityName, exampleRowPrefix + " Salida", d(2), d(3), statusCoded.label("not_started"), datos},
	}
}

// writeGanttSheet arma el Gantt: una tabla de fórmulas que lee la hoja
// Entregables (tarea, inicio, fin, desplazamiento y duración en días) y un
// gráfico de barras apiladas donde la primera serie es invisible, así la
// segunda arranca en la fecha de inicio de cada entregable. Es la forma de
// hacer un Gantt en Excel sin macros: las fórmulas se recalculan solas a
// medida que se llena la hoja Entregables.
func (x *xw) writeGanttSheet(dlvCols []xlsxColumn, st templateStyles) {
	actCol := letterOf(dlvCols, "Actividad")
	nameCol := letterOf(dlvCols, "Entregable")
	startCol := letterOf(dlvCols, "Inicio")
	endCol := letterOf(dlvCols, "Fin")
	statusCol := letterOf(dlvCols, "Estado")

	x.hideGridLines(sheetGantt)
	x.cell(sheetGantt, "A1", "Gantt de entregables")
	x.style(sheetGantt, "A1", "A1", st.title)
	x.merge(sheetGantt, "A2", "F2")
	x.cell(sheetGantt, "A2", "Se calcula solo con las columnas Inicio y Fin de la hoja Entregables. Si cambias una fecha allá, la barra se mueve acá.")
	x.style(sheetGantt, "A2", "F2", st.lead)
	x.rowHeight(sheetGantt, 1, 26)
	x.rowHeight(sheetGantt, 2, 18)

	const tableHeaderRow = 4
	firstRow := tableHeaderRow + 1
	lastRow := tableHeaderRow + ganttRows

	headers := []string{"Tarea", "Inicio", "Fin", "Estado", "Inicio (auxiliar, días)", "Duración (días)"}
	for i, h := range headers {
		letter := colLetter(i + 1)
		x.cell(sheetGantt, fmt.Sprintf("%s%d", letter, tableHeaderRow), h)
		x.style(sheetGantt, fmt.Sprintf("%s%d", letter, tableHeaderRow), fmt.Sprintf("%s%d", letter, tableHeaderRow),
			st.header[groupDlv])
	}
	x.rowHeight(sheetGantt, tableHeaderRow, 30)
	x.width(sheetGantt, "A", "A", 48)
	x.width(sheetGantt, "B", "C", 13)
	x.width(sheetGantt, "D", "D", 16)
	x.width(sheetGantt, "E", "F", 21)

	x.cell(sheetGantt, "H4", "Inicio del portafolio")
	x.style(sheetGantt, "H4", "H4", st.label)
	x.width(sheetGantt, "H", "H", 22)
	x.formula(sheetGantt, "H5", fmt.Sprintf(`IF(COUNT($B$%d:$B$%d)=0,0,MIN($B$%d:$B$%d))`,
		firstRow, lastRow, firstRow, lastRow))
	x.style(sheetGantt, "H5", "H5", st.date)

	for i := range ganttRows {
		row := firstRow + i
		src := firstDataRow + i // fila correspondiente en la hoja Entregables
		x.formula(sheetGantt, fmt.Sprintf("A%d", row), fmt.Sprintf(
			`IF(%[1]s!%[2]s%[4]d="","",%[1]s!%[3]s%[4]d&" · "&%[1]s!%[2]s%[4]d)`,
			sheetDeliverables, nameCol, actCol, src))
		x.formula(sheetGantt, fmt.Sprintf("B%d", row), fmt.Sprintf(
			`IF(%[1]s!%[2]s%[5]d="","",IF(%[1]s!%[3]s%[5]d="",%[1]s!%[4]s%[5]d,%[1]s!%[3]s%[5]d))`,
			sheetDeliverables, nameCol, startCol, endCol, src))
		x.formula(sheetGantt, fmt.Sprintf("C%d", row), fmt.Sprintf(
			`IF(%[1]s!%[2]s%[4]d="","",%[1]s!%[3]s%[4]d)`,
			sheetDeliverables, nameCol, endCol, src))
		x.formula(sheetGantt, fmt.Sprintf("D%d", row), fmt.Sprintf(
			`IF(%[1]s!%[2]s%[4]d="","",%[1]s!%[3]s%[4]d)`,
			sheetDeliverables, nameCol, statusCol, src))
		x.formula(sheetGantt, fmt.Sprintf("E%d", row), fmt.Sprintf(`IF(B%[1]d="",NA(),B%[1]d-$H$5)`, row))
		x.formula(sheetGantt, fmt.Sprintf("F%d", row), fmt.Sprintf(`IF(B%[1]d="",NA(),MAX(1,C%[1]d-B%[1]d))`, row))
	}
	x.style(sheetGantt, fmt.Sprintf("B%d", firstRow), fmt.Sprintf("C%d", lastRow), st.date)
	x.style(sheetGantt, fmt.Sprintf("D%d", firstRow), fmt.Sprintf("D%d", lastRow), st.helper)
	x.style(sheetGantt, fmt.Sprintf("E%d", firstRow), fmt.Sprintf("F%d", lastRow), st.helper)
	x.do(x.f.SetConditionalFormat(sheetGantt,
		fmt.Sprintf("A%d:F%d", firstRow, lastRow),
		[]excelize.ConditionalFormatOptions{
			{Type: "formula", Criteria: "=MOD(ROW(),2)=0", Format: &st.cfZebra},
		}))

	rng := func(col string) string {
		return fmt.Sprintf("%s!$%s$%d:$%s$%d", sheetGantt, col, firstRow, col, lastRow)
	}
	x.do(x.f.AddChart(sheetGantt, "H8", &excelize.Chart{
		Type: excelize.BarStacked,
		Series: []excelize.ChartSeries{
			{
				// El nombre va como referencia a la celda del encabezado: un
				// nombre literal viaja como fórmula inválida y la leyenda acaba
				// diciendo "Column E".
				Name:       fmt.Sprintf("%s!$E$%d", sheetGantt, tableHeaderRow),
				Categories: rng("A"),
				Values:     rng("E"),
				// La barra de desplazamiento existe para empujar la barra de
				// duración, no para verse: va en blanco sobre el área blanca del
				// gráfico. "Sin relleno" sería lo natural, pero LibreOffice
				// igual le pinta un tinte de su paleta.
				Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFFF"}},
			},
			{
				Name:       fmt.Sprintf("%s!$F$%d", sheetGantt, tableHeaderRow),
				Categories: rng("A"),
				Values:     rng("F"),
				Fill:       excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{svg.SectionPalette[2]}},
			},
		},
		// Fondo blanco (marco y área de trazado): sin relleno propio, las
		// franjas alternas de la hoja se transparentan bajo el gráfico.
		Fill:     excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFFF"}},
		PlotArea: excelize.ChartPlotArea{Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFFF"}}},
		Title:    excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Entregables en el tiempo"}}},
		Legend:   excelize.ChartLegend{Position: "bottom"},
		// En un gráfico de barras horizontales el eje de categorías (XAxis en
		// excelize) es el de las tareas y el de valores (YAxis) el de días: se
		// invierte el de categorías para que el primer entregable quede arriba.
		XAxis: excelize.ChartAxis{ReverseOrder: true, Font: excelize.Font{Size: 9}},
		YAxis: excelize.ChartAxis{
			Title:          excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Días desde el inicio del portafolio"}}},
			MajorGridLines: true,
		},
		// Sin varyColors todas las barras de duración comparten color; con él
		// Excel puede pintar cada punto de un color distinto.
		VaryColors:   new(false),
		Dimension:    excelize.ChartDimension{Width: 980, Height: 660},
		ShowBlanksAs: "gap",
	}))
}

// writeInstructionsSheet escribe la portada/guía. El glosario de columnas se
// genera desde las definiciones de xlsx_fields.go, así no queda desfasado si se
// agrega o cambia una pregunta.
func (x *xw) writeInstructionsSheet(actCols, dlvCols []xlsxColumn, sections []string, st templateStyles) {
	x.hideGridLines(sheetInstructions)
	x.width(sheetInstructions, "A", "A", 3)
	x.width(sheetInstructions, "B", "B", 16)
	x.width(sheetInstructions, "C", "C", 46)
	x.width(sheetInstructions, "D", "D", 12)
	x.width(sheetInstructions, "E", "E", 86)

	row := 2
	put := func(col string, v any, styleID int) {
		cell := fmt.Sprintf("%s%d", col, row)
		x.cell(sheetInstructions, cell, v)
		x.style(sheetInstructions, cell, cell, styleID)
	}
	// banner ocupa el ancho útil de la hoja con el color de la app.
	banner := func(text string, styleID int, height float64) {
		x.merge(sheetInstructions, fmt.Sprintf("B%d", row), fmt.Sprintf("E%d", row))
		put("B", text, styleID)
		x.rowHeight(sheetInstructions, row, height)
		row++
	}
	line := func(text string) {
		x.merge(sheetInstructions, fmt.Sprintf("B%d", row), fmt.Sprintf("E%d", row))
		put("B", text, st.body)
		row++
	}

	banner("Catchup · carga masiva de actividades", st.title, 30)
	banner("Llena las hojas Actividades y Entregables, revisa el Gantt y sube el archivo en Catchup. "+
		"Las celdas con desplegable no aceptan otros valores, y la columna «¿Fila completa?» avisa si falta algo.", st.lead, 30)
	row++

	banner("Cómo se llena", st.subtitle, 20)
	for _, t := range []string{
		"1. Hoja «Actividades»: una fila por actividad. Las columnas con * son obligatorias; el resto es opcional.",
		"2. Cada celda con desplegable muestra las opciones exactas del cuestionario, incluida la escala 1–5 con su descripción.",
		"3. Hoja «Entregables»: una fila por entregable. La columna Actividad es un desplegable con los nombres que escribiste en la hoja anterior.",
		"4. Hoja «Gantt»: se arma sola con las columnas Inicio y Fin de Entregables. No hay que tocar nada.",
		"5. Guarda y sube el archivo en Catchup: Configuración → «Cargar actividades desde Excel».",
	} {
		line(t)
	}
	row++

	banner("Reglas de la carga", st.subtitle, 20)
	for _, t := range []string{
		"• Las filas sin Nombre (o sin Entregable) se ignoran.",
		"• Las filas de ejemplo en gris cursiva (empiezan con «" + exampleRowPrefix + "») se omiten al importar: puedes borrarlas o dejarlas.",
		"• La validación es todo-o-nada: si hay algún error no se importa nada y Catchup lista todos los errores con su número de fila.",
		"• Las columnas grises de la derecha («¿Fila completa?», «Duración») son calculadas y Catchup las ignora.",
		"• También se aceptan las claves internas (Project, none, breach…), para reeditar y volver a subir un archivo bajado con «Descargar Excel».",
		"• Cada carga crea actividades nuevas: no actualiza ni reemplaza las existentes.",
		"• Las fechas pueden ir como fecha de Excel o como texto AAAA-MM-DD.",
		"• La columna «Inicio» de Entregables solo alimenta el Gantt; Catchup guarda la fecha «Fin» como fecha del entregable.",
		"• El gráfico de Gantt está cableado a las primeras " + fmt.Sprint(ganttRows) + " filas de Entregables; si necesitas más, extiende el rango del gráfico.",
		"• Secciones configuradas hoy en Catchup: " + strings.Join(sections, ", ") + ".",
	} {
		line(t)
	}
	row++

	glossary := func(title string, cols []xlsxColumn) {
		banner(title, st.subtitle, 20)
		for col, head := range map[string]string{"B": "Columna", "C": "Pregunta", "D": "Obligatorio", "E": "Valores válidos y notas"} {
			put(col, head, st.header[cols[0].Group])
		}
		x.rowHeight(sheetInstructions, row, 24)
		row++
		for _, c := range cols {
			put("B", c.Header, st.labelTop)
			put("C", c.Question, st.wrap)
			required := "no"
			if c.Required {
				required = "sí"
			}
			put("D", required, st.wrap)
			notes := c.validValuesText()
			if c.Help != "" {
				notes += ". " + c.Help
			}
			put("E", notes, st.wrap)
			row++
		}
		row++
	}
	glossary("Hoja «Actividades» — columna por columna", actCols)
	glossary("Hoja «Entregables» — columna por columna", dlvCols)
}
