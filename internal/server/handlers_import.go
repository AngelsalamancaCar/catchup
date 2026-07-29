package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"projectmapper/internal/model"
)

// maxImportSize acota el tamaño del archivo subido a /import.
const maxImportSize = 10 << 20 // 10 MiB

var validActivityTypes = map[string]bool{"Project": true, "Workstream": true, "Recurring": true, "AdHoc": true}

// importPayload es el formato que emite scripts/import_backlog.py: puede ser
// solo {"activities": [...]}, o el Store completo, del que solo se usa el
// campo .activities (sections/weights/thresholds del archivo se ignoran acá
// para no pisar la configuración existente al importar sobre un store real).
type importPayload struct {
	Activities []model.Activity `json:"activities"`
}

// handleImport recibe un archivo JSON subido vía formulario multipart
// (campo "file") y crea cada actividad vía Store.CreateActivity, que asigna
// ID y timestamps nuevos e ignora los que traiga el archivo — §3 Fase 4,
// tarea 4 ("se importa vía POST /import o escribiendo el archivo": la otra
// mitad de esa opcionalidad es simplemente reemplazar el JSON de datos a
// mano, lo cual ya funciona sin ningún código nuevo).
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxImportSize); err != nil {
		s.renderImportError(w, "Formulario inválido: "+err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		s.renderImportError(w, "Selecciona un archivo JSON para importar.")
		return
	}
	defer file.Close()

	b, err := io.ReadAll(io.LimitReader(file, maxImportSize+1))
	if err != nil || len(b) > maxImportSize {
		s.renderImportError(w, "No se pudo leer el archivo (¿demasiado grande?).")
		return
	}

	var payload importPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		s.renderImportError(w, "JSON inválido: "+err.Error())
		return
	}

	imported := 0
	var skipped []string
	for _, a := range payload.Activities {
		if strings.TrimSpace(a.Name) == "" {
			skipped = append(skipped, "(sin nombre)")
			continue
		}
		if !validActivityTypes[a.Type] {
			a.Type = "AdHoc"
		}
		if _, err := s.store.CreateActivity(a); err != nil {
			skipped = append(skipped, a.Name)
			continue
		}
		imported++
	}

	data := s.configPageData()
	data.ImportResult = fmt.Sprintf("Importadas %d actividad(es).", imported)
	if len(skipped) > 0 {
		data.ImportResult += fmt.Sprintf(" Omitidas %d: %s.", len(skipped), strings.Join(skipped, ", "))
	}
	s.renderFull(w, "config", data)
}

func (s *Server) renderImportError(w http.ResponseWriter, msg string) {
	data := s.configPageData()
	data.ImportErr = msg
	w.WriteHeader(http.StatusUnprocessableEntity)
	s.renderFull(w, "config", data)
}
