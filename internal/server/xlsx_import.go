package server

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"projectmapper/internal/model"
)

// dateLayouts son los formatos de texto aceptados en las columnas de fecha.
// Una fecha real de Excel llega como número de serie y no pasa por acá; estos
// layouts cubren el caso de celdas escritas como texto (el orden día/mes sigue
// la convención de la UI, dd/mm/aaaa).
var dateLayouts = []string{"2006-01-02", "2006/01/02", "02/01/2006", "02-01-2006", "2006-01-02 15:04:05"}

// xlsxImportOutcome es el resultado de leer el Excel de carga masiva. Las
// actividades solo se escriben en el store si Errors está vacío: una carga
// masiva a medias es más difícil de arreglar que un archivo rechazado.
type xlsxImportOutcome struct {
	Activities []model.Activity
	Errors     []string
	Warnings   []string
	Skipped    int // filas de ejemplo omitidas
}

// hasSheet reporta si el libro trae la hoja dada (GetSheetIndex devuelve -1
// cuando no existe).
func hasSheet(f *excelize.File, name string) bool {
	i, err := f.GetSheetIndex(name)
	return err == nil && i >= 0
}

// normalizeHeader deja un encabezado comparable, tolerando el " *" que marca
// las columnas obligatorias en el template.
func normalizeHeader(s string) string {
	return normalizeCell(strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "*")), " "))
}

// headerScanRows es cuántas filas del principio de la hoja se revisan buscando
// los encabezados: el template los pone en la fila 2 (debajo de la banda de
// grupos) y /export/xlsx en la fila 1, y alguien puede agregar un título arriba.
const headerScanRows = 8

// findHeaderRow devuelve el índice (base 0) de la fila que mejor coincide con
// los encabezados esperados, junto con su mapeo y las columnas obligatorias que
// falten. Buscar la fila en vez de fijarla permite cambiar la presentación del
// template sin romper los archivos ya repartidos.
func findHeaderRow(rows [][]string, cols []xlsxColumn) (int, map[string]int, []string) {
	best, bestIdx, bestMissing := -1, map[string]int{}, []string{}
	limit := min(len(rows), headerScanRows)
	for i := range limit {
		idx, missing := headerIndex(rows[i], cols)
		if len(idx) > len(bestIdx) || best < 0 {
			best, bestIdx, bestMissing = i, idx, missing
		}
	}
	return best, bestIdx, bestMissing
}

// headerIndex mapea encabezado → índice de columna, y reporta las columnas
// obligatorias que falten en la hoja.
func headerIndex(header []string, cols []xlsxColumn) (map[string]int, []string) {
	idx := map[string]int{}
	for i, raw := range header {
		idx[normalizeHeader(raw)] = i
	}
	out := map[string]int{}
	var missing []string
	for _, c := range cols {
		i, ok := idx[normalizeHeader(c.Header)]
		if !ok {
			if c.Required {
				missing = append(missing, c.Header)
			}
			continue
		}
		out[c.Header] = i
	}
	return out, missing
}

// cellReader lee una fila de la hoja por nombre de columna y acumula los
// errores/avisos ya prefijados con hoja y número de fila.
type cellReader struct {
	sheet  string
	rowNum int
	idx    map[string]int
	row    []string
	errs   []string
	warns  []string
}

func (c *cellReader) raw(header string) string {
	i, ok := c.idx[header]
	if !ok || i >= len(c.row) {
		return ""
	}
	return strings.TrimSpace(c.row[i])
}

func (c *cellReader) fail(header, msg string) {
	c.errs = append(c.errs, fmt.Sprintf("%s fila %d, columna %s: %s", c.sheet, c.rowNum, header, msg))
}

func (c *cellReader) warn(header, msg string) {
	c.warns = append(c.warns, fmt.Sprintf("%s fila %d, columna %s: %s", c.sheet, c.rowNum, header, msg))
}

func (c *cellReader) text(header string, required bool) string {
	v := c.raw(header)
	if v == "" && required {
		c.fail(header, "es obligatorio.")
	}
	return v
}

func (c *cellReader) coded(header string, f codedField, required bool) string {
	raw := c.raw(header)
	if raw == "" {
		if required {
			c.fail(header, "es obligatorio. Valores: "+strings.Join(f.labels, " · "))
		}
		return ""
	}
	key, ok := f.resolve(raw)
	if !ok {
		c.fail(header, fmt.Sprintf("%q no es un valor válido. Valores: %s", raw, strings.Join(f.labels, " · ")))
		return ""
	}
	return key
}

func (c *cellReader) scale(header string) int {
	raw := c.raw(header)
	if raw == "" {
		c.fail(header, "es obligatorio (escala 1 a 5).")
		return 0
	}
	v, ok := parseScaleCell(raw)
	if !ok {
		c.fail(header, fmt.Sprintf("%q no es un valor de 1 a 5.", raw))
		return 0
	}
	return v
}

// parseScaleCell lee una celda de escala 1-5. El desplegable del template
// guarda la opción completa ("4 — Apoya un objetivo estratégico principal"), y
// /export/xlsx guarda el número solo: se toma el número inicial en ambos casos.
func parseScaleCell(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		if v != float64(int(v)) || !in1to5(int(v)) {
			return 0, false
		}
		return int(v), true
	}
	digits := raw
	if i := strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		digits = raw[:i]
	}
	v, err := strconv.Atoi(digits)
	if err != nil || !in1to5(v) {
		return 0, false
	}
	return v, true
}

func (c *cellReader) date(header string, required bool) string {
	raw := c.raw(header)
	if raw == "" {
		if required {
			c.fail(header, "es obligatoria (fecha AAAA-MM-DD).")
		}
		return ""
	}
	iso, err := parseXLSXDate(raw)
	if err != nil {
		c.fail(header, fmt.Sprintf("%q no es una fecha válida (usa AAAA-MM-DD).", raw))
		return ""
	}
	return iso
}

// splitList parte un campo multivalor (";" o "," como separador).
func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == ',' || r == '\n' }) {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseXLSXDate acepta el número de serie de Excel o una fecha en texto, y
// devuelve la fecha ISO que usa el modelo.
func parseXLSXDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if serial, err := strconv.ParseFloat(raw, 64); err == nil {
		if serial < 1 {
			return "", fmt.Errorf("fecha fuera de rango: %s", raw)
		}
		t, err := excelize.ExcelDateToTime(serial, false)
		if err != nil {
			return "", err
		}
		return t.Format("2006-01-02"), nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("formato de fecha no reconocido: %s", raw)
}

// isExampleRow reporta si el valor viene de una fila de ejemplo del template.
func isExampleRow(name string) bool {
	return strings.HasPrefix(normalizeCell(name), normalizeCell(exampleRowPrefix))
}

func rowIsEmpty(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// parseActivityRow convierte una fila de la hoja Actividades en model.Activity.
func parseActivityRow(idx map[string]int, row []string, rowNum int, sections []string) (model.Activity, []string, []string) {
	c := &cellReader{sheet: sheetActivities, rowNum: rowNum, idx: idx, row: row}

	a := model.Activity{
		Name:        c.text("Nombre", true),
		Type:        c.coded("Tipo", typeCoded, true),
		Owner:       c.text("Owner", true),
		Section:     c.text("Sección", true),
		Description: c.text("Descripción", false),
	}
	if a.Section != "" && !contains(sections, a.Section) {
		c.warn("Sección", fmt.Sprintf("%q no está configurada en Catchup; la actividad quedará en el carril \"sin sección\".", a.Section))
	}
	for _, sec := range splitList(c.raw("Involucradas")) {
		if !contains(sections, sec) {
			c.warn("Involucradas", fmt.Sprintf("%q no es una sección configurada; se guarda igual.", sec))
		}
		a.Involved = append(a.Involved, sec)
	}

	a.Answers = model.Answers{
		B1: c.scale("B1"),
		B2: c.scale("B2"),
		B3: c.scale("B3"),
		B4: c.scale("B4"),
		C1: c.coded("C1", c1Coded, true),
		C2: c.scale("C2"),
		C3: c.scale("C3"),
		C4: c.coded("C4", c4Coded, true),
		D2: c.coded("D2", d2Coded, true),
		D3: c.coded("D3", d3Coded, true),
		D4: c.coded("D4", d4Coded, true),
		F1: c.scale("F1"),
		F3: c.text("F3", false),
	}
	if d1 := c.date("D1", false); d1 != "" {
		a.Answers.D1 = &d1
	}
	for _, key := range splitList(c.raw("F2")) {
		k := strings.ToUpper(strings.TrimSpace(key))
		if !contains(f2Keys, k) {
			c.fail("F2", fmt.Sprintf("%q no es un código de pregunta válido. Válidos: %s", key, strings.Join(f2Keys, ", ")))
			continue
		}
		a.Answers.F2 = append(a.Answers.F2, k)
	}

	return a, c.errs, c.warns
}

// pendingDeliverable es un entregable leído de la hoja Entregables, antes de
// resolver a qué actividad pertenece y a qué ID apunta su dependencia.
type pendingDeliverable struct {
	activity  string
	name      string
	due       string
	status    string
	dependsOn string
	rowNum    int
}

// parseImportWorkbook lee las hojas Actividades y Entregables del archivo
// subido. No toca el store: devuelve las actividades listas para crear, o la
// lista completa de errores por fila si el archivo tiene algo mal.
func parseImportWorkbook(f *excelize.File, sections []string) xlsxImportOutcome {
	var out xlsxImportOutcome

	if !hasSheet(f, sheetActivities) {
		out.Errors = append(out.Errors, fmt.Sprintf(
			"El archivo no tiene una hoja llamada %q. Descarga el template y usa esa estructura.", sheetActivities))
		return out
	}
	rows, err := f.GetRows(sheetActivities, excelize.Options{RawCellValue: true})
	if err != nil {
		out.Errors = append(out.Errors, "No se pudo leer la hoja "+sheetActivities+": "+err.Error())
		return out
	}
	if len(rows) == 0 {
		out.Errors = append(out.Errors, "La hoja "+sheetActivities+" está vacía.")
		return out
	}

	header, idx, missing := findHeaderRow(rows, activityColumns())
	if len(missing) > 0 {
		out.Errors = append(out.Errors, fmt.Sprintf(
			"A la hoja %s le faltan columnas obligatorias: %s.", sheetActivities, strings.Join(missing, ", ")))
		return out
	}

	// byName resuelve la referencia por nombre que usa la hoja Entregables.
	byName := map[string]int{}
	for i, row := range rows[header+1:] {
		rowNum := header + i + 2
		if rowIsEmpty(row) {
			continue
		}
		a, errs, warns := parseActivityRow(idx, row, rowNum, sections)
		if isExampleRow(a.Name) {
			out.Skipped++
			continue
		}
		out.Errors = append(out.Errors, errs...)
		out.Warnings = append(out.Warnings, warns...)

		key := normalizeCell(a.Name)
		if _, dup := byName[key]; dup && key != "" {
			out.Errors = append(out.Errors, fmt.Sprintf(
				"%s fila %d, columna Nombre: %q está repetido; los nombres deben ser únicos para poder ligar los entregables.",
				sheetActivities, rowNum, a.Name))
			continue
		}
		byName[key] = len(out.Activities)
		out.Activities = append(out.Activities, a)
	}

	if len(out.Activities) == 0 && len(out.Errors) == 0 {
		out.Errors = append(out.Errors, "La hoja "+sheetActivities+" no tiene ninguna actividad para importar.")
		return out
	}

	pending, errs, warns, skipped := readDeliverableRows(f)
	out.Errors = append(out.Errors, errs...)
	out.Warnings = append(out.Warnings, warns...)
	out.Skipped += skipped
	out.attachDeliverables(pending, byName)

	return out
}

// readDeliverableRows lee la hoja Entregables (opcional) sin resolver todavía
// la actividad a la que pertenece cada fila.
func readDeliverableRows(f *excelize.File) (pending []pendingDeliverable, errs, warns []string, skipped int) {
	if !hasSheet(f, sheetDeliverables) {
		return nil, nil, nil, 0
	}
	rows, err := f.GetRows(sheetDeliverables, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, []string{"No se pudo leer la hoja " + sheetDeliverables + ": " + err.Error()}, nil, 0
	}
	if len(rows) == 0 {
		return nil, nil, nil, 0
	}
	header, idx, missing := findHeaderRow(rows, deliverableColumns())
	if len(missing) > 0 {
		return nil, []string{fmt.Sprintf("A la hoja %s le faltan columnas obligatorias: %s.",
			sheetDeliverables, strings.Join(missing, ", "))}, nil, 0
	}

	for i, row := range rows[header+1:] {
		rowNum := header + i + 2
		if rowIsEmpty(row) {
			continue
		}
		c := &cellReader{sheet: sheetDeliverables, rowNum: rowNum, idx: idx, row: row}
		d := pendingDeliverable{
			activity:  c.text("Actividad", true),
			name:      c.text("Entregable", true),
			status:    c.coded("Estado", statusCoded, false),
			dependsOn: c.raw("Depende de"),
			rowNum:    rowNum,
		}
		if isExampleRow(d.activity) || isExampleRow(d.name) {
			skipped++
			continue
		}
		d.due = c.date("Fin", true)
		if d.status == "" {
			d.status = "not_started"
		}
		errs = append(errs, c.errs...)
		warns = append(warns, c.warns...)
		pending = append(pending, d)
	}
	return pending, errs, warns, skipped
}

// attachDeliverables liga cada fila de Entregables con su actividad, asigna los
// IDs DLV-NNN (por orden de aparición dentro de la actividad) y traduce la
// columna "Depende de" de nombre a ID.
func (out *xlsxImportOutcome) attachDeliverables(pending []pendingDeliverable, byName map[string]int) {
	// grouped mantiene, por actividad, los entregables en orden de hoja.
	grouped := map[int][]pendingDeliverable{}
	for _, d := range pending {
		if d.activity == "" || d.name == "" {
			continue // ya reportado como error de fila
		}
		ai, ok := byName[normalizeCell(d.activity)]
		if !ok {
			out.Errors = append(out.Errors, fmt.Sprintf(
				"%s fila %d, columna Actividad: %q no aparece en la hoja %s.",
				sheetDeliverables, d.rowNum, d.activity, sheetActivities))
			continue
		}
		grouped[ai] = append(grouped[ai], d)
	}

	// El orden de recorrido de un map no es estable: se recorre por índice de
	// actividad para que dos cargas del mismo archivo reporten los mismos
	// errores en el mismo orden.
	for _, ai := range slices.Sorted(maps.Keys(grouped)) {
		list := grouped[ai]
		ids := map[string]string{} // nombre normalizado → DLV-NNN
		for i, d := range list {
			ids[normalizeCell(d.name)] = fmt.Sprintf("DLV-%03d", i+1)
		}
		for i, d := range list {
			dlv := model.Deliverable{
				ID:     fmt.Sprintf("DLV-%03d", i+1),
				Name:   d.name,
				Due:    d.due,
				Status: d.status,
			}
			if d.dependsOn != "" {
				depID, ok := ids[normalizeCell(d.dependsOn)]
				switch {
				case !ok:
					out.Errors = append(out.Errors, fmt.Sprintf(
						"%s fila %d, columna Depende de: %q no es un entregable de %q.",
						sheetDeliverables, d.rowNum, d.dependsOn, d.activity))
				case depID == dlv.ID:
					out.Errors = append(out.Errors, fmt.Sprintf(
						"%s fila %d, columna Depende de: un entregable no puede depender de sí mismo.",
						sheetDeliverables, d.rowNum))
				default:
					dlv.DependsOn = &depID
				}
			}
			out.Activities[ai].Deliverables = append(out.Activities[ai].Deliverables, dlv)
		}
	}
}
