package server

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

var testSections = []string{"Operaciones", "Finanzas", "IT"}

// newTestWorkbook arma un libro mínimo con la estructura del template (solo
// encabezados y las filas dadas), para probar el parser sin depender del
// formato/validaciones del template completo.
func newTestWorkbook(t *testing.T, activities, deliverables [][]any) *excelize.File {
	t.Helper()
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", sheetActivities); err != nil {
		t.Fatalf("SetSheetName: %v", err)
	}
	if _, err := f.NewSheet(sheetDeliverables); err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	write := func(sheet string, cols []xlsxColumn, rows [][]any) {
		header := make([]any, len(cols))
		for i, c := range cols {
			header[i] = c.Header
			if c.Required {
				header[i] = c.Header + " *"
			}
		}
		if err := f.SetSheetRow(sheet, "A1", &header); err != nil {
			t.Fatalf("encabezados de %s: %v", sheet, err)
		}
		for i, row := range rows {
			if err := f.SetSheetRow(sheet, fmt.Sprintf("A%d", i+2), &row); err != nil {
				t.Fatalf("fila %d de %s: %v", i+2, sheet, err)
			}
		}
	}
	write(sheetActivities, activityColumns(), activities)
	write(sheetDeliverables, deliverableColumns(), deliverables)
	t.Cleanup(func() { f.Close() })
	return f
}

// validActivityRow es una fila completa y válida de la hoja Actividades, con
// los valores tal como los ofrecen los desplegables del template.
func validActivityRow(name string) []any {
	return []any{
		name, "Proyecto", "Marta", "Finanzas", "IT", "Reemplazar el ERP",
		5, 4, 5, 4,
		"60 a 120", 4, 4, "Algunas",
		"2026-11-30", "Escalamiento", "Compartidos", "No",
		3, "B3;C1", "Falta el alcance de tesorería",
	}
}

func firstError(t *testing.T, out xlsxImportOutcome) string {
	t.Helper()
	if len(out.Errors) == 0 {
		t.Fatalf("esperaba al menos un error, no hubo ninguno")
	}
	return strings.Join(out.Errors, " | ")
}

func TestBuildImportTemplateWorkbookStructure(t *testing.T) {
	f, err := buildImportTemplateWorkbook(testSections, time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildImportTemplateWorkbook: %v", err)
	}
	defer f.Close()

	for _, sheet := range []string{sheetInstructions, sheetActivities, sheetDeliverables, sheetGantt, sheetLists} {
		if !slices.Contains(f.GetSheetList(), sheet) {
			t.Errorf("falta la hoja %q; hojas: %v", sheet, f.GetSheetList())
		}
	}

	// Los encabezados son la interfaz del import: si cambian de fila o de
	// nombre, el parser deja de encontrar las columnas.
	rows, err := f.GetRows(sheetActivities)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	header, idx, missing := findHeaderRow(rows, activityColumns())
	if len(missing) > 0 {
		t.Errorf("el template no trae las columnas obligatorias %v", missing)
	}
	if len(idx) != len(activityColumns()) {
		t.Errorf("mapeé %d columnas de %d en el template", len(idx), len(activityColumns()))
	}
	if header != headerRow-1 {
		t.Errorf("los encabezados quedaron en la fila %d, esperaba la %d (debajo de la banda de grupos)", header+1, headerRow)
	}

	// La banda de grupos es lo que da la lectura de formulario a la hoja.
	if band, _ := f.GetCellValue(sheetActivities, "A1"); band != groupIdent {
		t.Errorf("A1 = %q, esperaba la banda %q", band, groupIdent)
	}
	// Columna calculada de apoyo, a la derecha de los datos.
	statusCol := colLetter(len(activityColumns()) + 1)
	if h, _ := f.GetCellValue(sheetActivities, fmt.Sprintf("%s%d", statusCol, headerRow)); h != "¿Fila completa?" {
		t.Errorf("encabezado del semáforo = %q, esperaba \"¿Fila completa?\"", h)
	}
	if got, _ := f.GetCellFormula(sheetActivities, fmt.Sprintf("%s%d", statusCol, firstDataRow)); !strings.Contains(got, "COUNTBLANK") {
		t.Errorf("el semáforo de la primera fila = %q, esperaba una fórmula con COUNTBLANK", got)
	}

	visible, err := f.GetSheetVisible(sheetLists)
	if err != nil {
		t.Fatalf("GetSheetVisible: %v", err)
	}
	if visible {
		t.Errorf("la hoja %s debería quedar oculta", sheetLists)
	}

	// El Gantt vive de fórmulas que leen la hoja Entregables.
	formula, err := f.GetCellFormula(sheetGantt, "E5")
	if err != nil {
		t.Fatalf("GetCellFormula: %v", err)
	}
	if formula == "" {
		t.Errorf("Gantt!E5 debería traer la fórmula de desplazamiento en días")
	}
	if got, _ := f.GetCellFormula(sheetGantt, "B5"); !strings.Contains(got, sheetDeliverables) {
		t.Errorf("Gantt!B5 = %q, esperaba una fórmula que lea la hoja %s", got, sheetDeliverables)
	}
}

// TestTemplateScaleDropdownCarriesAnchors comprueba que el desplegable de una
// pregunta 1-5 no ofrezca números pelados: la descripción de cada punto es lo
// que evita que se llene la escala a ojo.
func TestTemplateScaleDropdownCarriesAnchors(t *testing.T) {
	f, err := buildImportTemplateWorkbook(testSections, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildImportTemplateWorkbook: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows(sheetLists)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	var found string
	for _, row := range rows[1:] {
		for _, cell := range row {
			if strings.HasPrefix(cell, "4 — ") {
				found = cell
			}
		}
	}
	if found == "" {
		t.Fatalf("la hoja %s no trae opciones de escala con su ancla (\"4 — …\")", sheetLists)
	}
	if v, ok := parseScaleCell(found); !ok || v != 4 {
		t.Errorf("parseScaleCell(%q) = %d,%v; el import debe quedarse con el número", found, v, ok)
	}
}

func TestImportTemplateRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	f, err := buildImportTemplateWorkbook(testSections, now)
	if err != nil {
		t.Fatalf("buildImportTemplateWorkbook: %v", err)
	}
	defer f.Close()

	// Se llena el template como lo haría el usuario: debajo de las filas de
	// ejemplo, que deben omitirse solas. La escala se llena con la opción
	// completa del desplegable, no con el número pelado.
	row := validActivityRow("Migración ERP")
	row[6] = "5 — Habilita directamente un objetivo top"
	if err := f.SetSheetRow(sheetActivities, fmt.Sprintf("A%d", firstDataRow+1), &row); err != nil {
		t.Fatalf("escribir actividad: %v", err)
	}
	dlvs := [][]any{
		{"Migración ERP", "Diseño funcional", "2026-08-01", "2026-09-15", "En progreso", ""},
		{"Migración ERP", "Salida a producción", "2026-09-16", "2026-11-30", "No iniciado", "Diseño funcional"},
	}
	for i, d := range dlvs {
		// Las 3 primeras filas de datos son los entregables de ejemplo.
		if err := f.SetSheetRow(sheetDeliverables, fmt.Sprintf("A%d", firstDataRow+3+i), &d); err != nil {
			t.Fatalf("escribir entregable: %v", err)
		}
	}

	out := parseImportWorkbook(f, testSections)
	if len(out.Errors) > 0 {
		t.Fatalf("no esperaba errores, obtuve: %v", out.Errors)
	}
	if len(out.Activities) != 1 {
		t.Fatalf("importé %d actividades, esperaba 1 (las de ejemplo se omiten)", len(out.Activities))
	}
	if out.Skipped != 4 {
		t.Errorf("Skipped = %d, esperaba 4 filas de ejemplo (1 actividad + 3 entregables)", out.Skipped)
	}

	a := out.Activities[0]
	if a.Name != "Migración ERP" || a.Type != "Project" || a.Section != "Finanzas" {
		t.Errorf("identificación mal traducida: %+v", a)
	}
	if a.Answers.C1 != "60-120" || a.Answers.C4 != "some" || a.Answers.D2 != "escalation" ||
		a.Answers.D3 != "shared" || a.Answers.D4 != "no" {
		t.Errorf("etiquetas no traducidas a claves del modelo: %+v", a.Answers)
	}
	if a.Answers.D1 == nil || *a.Answers.D1 != "2026-11-30" {
		t.Errorf("D1 = %v, esperaba 2026-11-30", a.Answers.D1)
	}
	if !slices.Equal(a.Answers.F2, []string{"B3", "C1"}) {
		t.Errorf("F2 = %v, esperaba [B3 C1]", a.Answers.F2)
	}
	if !slices.Equal(a.Involved, []string{"IT"}) {
		t.Errorf("Involved = %v, esperaba [IT]", a.Involved)
	}

	if len(a.Deliverables) != 2 {
		t.Fatalf("entregables = %d, esperaba 2", len(a.Deliverables))
	}
	if a.Deliverables[0].ID != "DLV-001" || a.Deliverables[0].Due != "2026-09-15" || a.Deliverables[0].Status != "in_progress" {
		t.Errorf("primer entregable mal leído: %+v", a.Deliverables[0])
	}
	dep := a.Deliverables[1].DependsOn
	if dep == nil || *dep != "DLV-001" {
		t.Errorf("DependsOn = %v, esperaba DLV-001 (resuelto por nombre)", dep)
	}
}

func TestImportAcceptsRawKeysAndExcelSerialDates(t *testing.T) {
	// Un archivo bajado con "Descargar Excel" trae claves crudas; y una fecha
	// escrita como fecha real de Excel llega como número de serie.
	row := validActivityRow("Reporte semanal")
	row[1] = "Recurring"
	row[10] = "5-20"
	row[13] = "none"
	row[14] = 46356 // 2026-11-30 en serie de Excel
	row[15] = "friction"
	row[16] = "mine"
	row[17] = "yes"

	out := parseImportWorkbook(newTestWorkbook(t, [][]any{row}, nil), testSections)
	if len(out.Errors) > 0 {
		t.Fatalf("no esperaba errores, obtuve: %v", out.Errors)
	}
	a := out.Activities[0]
	if a.Type != "Recurring" || a.Answers.C1 != "5-20" || a.Answers.C4 != "none" || a.Answers.D2 != "friction" {
		t.Errorf("claves crudas no aceptadas: %+v", a)
	}
	if a.Answers.D1 == nil || *a.Answers.D1 != "2026-11-30" {
		t.Errorf("D1 desde serie de Excel = %v, esperaba 2026-11-30", a.Answers.D1)
	}
}

func TestImportReportsRowErrorsAndImportsNothing(t *testing.T) {
	bad := validActivityRow("Sin owner")
	bad[2] = "" // Owner
	bad[6] = 9  // B1 fuera de escala
	bad[1] = "Proyecto grande"

	out := parseImportWorkbook(newTestWorkbook(t, [][]any{bad}, nil), testSections)
	msg := firstError(t, out)
	for _, want := range []string{"columna Owner", "columna B1", "columna Tipo", "fila 2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("los errores no mencionan %q: %s", want, msg)
		}
	}
}

func TestImportRejectsDeliverableWithoutActivity(t *testing.T) {
	out := parseImportWorkbook(newTestWorkbook(t,
		[][]any{validActivityRow("Migración ERP")},
		[][]any{{"Migracion ERPP", "Diseño", "", "2026-09-01", "En progreso", ""}},
	), testSections)

	if !strings.Contains(firstError(t, out), "no aparece en la hoja") {
		t.Errorf("esperaba un error de actividad inexistente, obtuve: %v", out.Errors)
	}
}

func TestImportRejectsUnknownDependency(t *testing.T) {
	out := parseImportWorkbook(newTestWorkbook(t,
		[][]any{validActivityRow("Migración ERP")},
		[][]any{{"Migración ERP", "Diseño", "", "2026-09-01", "En progreso", "Otro que no existe"}},
	), testSections)

	if !strings.Contains(firstError(t, out), "columna Depende de") {
		t.Errorf("esperaba un error de dependencia inexistente, obtuve: %v", out.Errors)
	}
}

func TestImportRejectsDuplicateActivityNames(t *testing.T) {
	out := parseImportWorkbook(newTestWorkbook(t,
		[][]any{validActivityRow("Migración ERP"), validActivityRow("migracion erp")},
		nil,
	), testSections)

	if !strings.Contains(firstError(t, out), "repetido") {
		t.Errorf("esperaba un error de nombre repetido, obtuve: %v", out.Errors)
	}
}

func TestImportWarnsOnUnknownSection(t *testing.T) {
	row := validActivityRow("Actividad huérfana")
	row[3] = "Marketing" // no está configurada
	row[4] = ""

	out := parseImportWorkbook(newTestWorkbook(t, [][]any{row}, nil), testSections)
	if len(out.Errors) > 0 {
		t.Fatalf("una sección desconocida no debería bloquear la carga: %v", out.Errors)
	}
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "Marketing") {
		t.Errorf("Warnings = %v, esperaba un aviso sobre Marketing", out.Warnings)
	}
	if out.Activities[0].Section != "Marketing" {
		t.Errorf("la sección debería guardarse tal cual, quedó %q", out.Activities[0].Section)
	}
}

func TestImportIgnoresEmptyRowsAndRequiresSomeActivity(t *testing.T) {
	out := parseImportWorkbook(newTestWorkbook(t, [][]any{{"", "", ""}}, nil), testSections)
	if len(out.Activities) != 0 {
		t.Errorf("las filas vacías no deberían generar actividades: %+v", out.Activities)
	}
	if !strings.Contains(firstError(t, out), "no tiene ninguna actividad") {
		t.Errorf("esperaba el aviso de archivo sin actividades, obtuve: %v", out.Errors)
	}
}

func TestImportRejectsMissingRequiredColumn(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", sheetActivities); err != nil {
		t.Fatalf("SetSheetName: %v", err)
	}
	if err := f.SetSheetRow(sheetActivities, "A1", &[]any{"Nombre", "Owner"}); err != nil {
		t.Fatalf("SetSheetRow: %v", err)
	}

	out := parseImportWorkbook(f, testSections)
	if !strings.Contains(firstError(t, out), "columnas obligatorias") {
		t.Errorf("esperaba el error de columnas faltantes, obtuve: %v", out.Errors)
	}
}

func TestParseXLSXDate(t *testing.T) {
	cases := []struct {
		raw, want string
		wantErr   bool
	}{
		{raw: "", want: ""},
		{raw: "2026-11-30", want: "2026-11-30"},
		{raw: "30/11/2026", want: "2026-11-30"},
		{raw: "2026/11/30", want: "2026-11-30"},
		{raw: "46356", want: "2026-11-30"},
		{raw: "el jueves", wantErr: true},
		{raw: "0", wantErr: true},
	}
	for _, c := range cases {
		got, err := parseXLSXDate(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseXLSXDate(%q) = %q, esperaba error", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseXLSXDate(%q): %v", c.raw, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseXLSXDate(%q) = %q, esperaba %q", c.raw, got, c.want)
		}
	}
}

func TestCodedFieldResolveIsForgiving(t *testing.T) {
	for _, raw := range []string{"Sí", "si", " SI ", "yes", "YES"} {
		if got, ok := d4Coded.resolve(raw); !ok || got != "yes" {
			t.Errorf("d4Coded.resolve(%q) = %q,%v; esperaba yes,true", raw, got, ok)
		}
	}
	if _, ok := d4Coded.resolve("tal vez"); ok {
		t.Errorf("d4Coded.resolve(\"tal vez\") debería fallar")
	}
}
