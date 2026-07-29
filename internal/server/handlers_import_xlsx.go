package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/xuri/excelize/v2"
)

// maxImportErrors acota la lista de errores que se muestra en pantalla cuando
// el archivo subido viene mal: con un template de 200 filas la lista completa
// no aporta más que las primeras.
const maxImportErrors = 40

// handleImportTemplate entrega el .xlsx de carga masiva, con las secciones
// configuradas ya cargadas en los desplegables de la hoja Actividades.
func (s *Server) handleImportTemplate(w http.ResponseWriter, r *http.Request) {
	f, err := buildImportTemplateWorkbook(s.store.Sections(), time.Now().UTC())
	if err != nil {
		http.Error(w, "error generando el template: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	const filename = "catchup_carga_actividades.xlsx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if _, err := f.WriteTo(w); err != nil {
		log.Printf("template xlsx: escribir respuesta: %v", err)
	}
}

// handleImportXLSX recibe el template lleno y crea las actividades. La carga es
// todo-o-nada: si alguna fila tiene errores no se escribe nada en el store y se
// devuelve la lista completa de problemas con su número de fila.
func (s *Server) handleImportXLSX(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxImportSize); err != nil {
		s.renderXLSXImportError(w, "Formulario inválido: "+err.Error(), nil)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		s.renderXLSXImportError(w, "Selecciona el archivo .xlsx a subir.", nil)
		return
	}
	defer file.Close()

	b, err := io.ReadAll(io.LimitReader(file, maxImportSize+1))
	if err != nil || len(b) > maxImportSize {
		s.renderXLSXImportError(w, "No se pudo leer el archivo (¿demasiado grande?).", nil)
		return
	}

	wb, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		s.renderXLSXImportError(w, "No es un archivo .xlsx válido: "+err.Error(), nil)
		return
	}
	defer wb.Close()

	outcome := parseImportWorkbook(wb, s.store.Sections())
	if len(outcome.Errors) > 0 {
		s.renderXLSXImportError(w,
			fmt.Sprintf("No se importó nada: el archivo tiene %d error(es).", len(outcome.Errors)),
			outcome.Errors)
		return
	}

	created := 0
	var failures []string
	for _, a := range outcome.Activities {
		if _, err := s.store.CreateActivity(a); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", a.Name, err))
			continue
		}
		created++
	}
	if len(failures) > 0 {
		s.renderXLSXImportError(w,
			fmt.Sprintf("Se crearon %d actividad(es) y %d fallaron al guardarse.", created, len(failures)),
			failures)
		return
	}

	data := s.configPageData()
	data.ImportResult = fmt.Sprintf("Importadas %d actividad(es) desde Excel, con %d entregable(s).",
		created, countDeliverables(outcome))
	if outcome.Skipped > 0 {
		data.ImportResult += fmt.Sprintf(" Se omitieron %d fila(s) de ejemplo.", outcome.Skipped)
	}
	data.ImportWarnings = trimList(outcome.Warnings)
	s.renderFull(w, "config", data)
}

func countDeliverables(outcome xlsxImportOutcome) int {
	n := 0
	for _, a := range outcome.Activities {
		n += len(a.Deliverables)
	}
	return n
}

// trimList corta la lista a maxImportErrors entradas y avisa cuántas quedaron fuera.
func trimList(list []string) []string {
	if len(list) <= maxImportErrors {
		return list
	}
	out := append([]string{}, list[:maxImportErrors]...)
	return append(out, fmt.Sprintf("… y %d más.", len(list)-maxImportErrors))
}

func (s *Server) renderXLSXImportError(w http.ResponseWriter, msg string, details []string) {
	data := s.configPageData()
	data.ImportErr = msg
	data.ImportErrors = trimList(details)
	w.WriteHeader(http.StatusUnprocessableEntity)
	s.renderFull(w, "config", data)
}
