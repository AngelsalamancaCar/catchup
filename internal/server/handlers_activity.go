package server

import (
	"net/http"
	"strconv"
)

// ListPageData es el contexto de la página de listado de actividades.
type ListPageData struct {
	Title      string
	Nav        string
	Activities any
}

func (s *Server) handleActivitiesList(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	s.renderFull(w, "activities_list", ListPageData{
		Title:      "Actividades",
		Nav:        "activities",
		Activities: snap.Activities,
	})
}

func (s *Server) handleWizardNew(w http.ResponseWriter, r *http.Request) {
	wf := WizardForm{
		Mode:     "new",
		Sections: s.store.Sections(),
		Errors:   map[string]string{},
		Title:    "Nueva actividad",
		Nav:      "activities",
	}
	s.renderFull(w, "activity_wizard", wf)
}

func (s *Server) handleWizardEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := s.store.GetActivity(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	wf := wizardFormFromActivity(a, s.store.Sections())
	wf.Title = "Editar actividad"
	wf.Nav = "activities"
	s.renderFull(w, "activity_wizard", wf)
}

// wizardAdvance construye un handler que valida el paso actual (currentStepTmpl)
// y, si es válido, renderiza el siguiente (nextStepTmpl). Si no es válido,
// re-renderiza el paso actual con errores inline sin perder lo tecleado.
func (s *Server) wizardAdvance(validate func(*WizardForm) bool, currentStepTmpl, nextStepTmpl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "formulario inválido", http.StatusBadRequest)
			return
		}
		wf := parseWizardForm(r, s.store.Sections())
		if !validate(&wf) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			s.renderFragment(w, "activity_wizard", currentStepTmpl, wf)
			return
		}
		s.renderFragment(w, "activity_wizard", nextStepTmpl, wf)
	}
}

// DeliverableRowData es el contexto para el botón "+ deliverable" (OOB swap).
type DeliverableRowData struct {
	NextIndex int
}

func (s *Server) handleDeliverableRow(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.URL.Query().Get("index"))
	if err != nil || idx < 0 || idx >= maxDeliverables {
		http.Error(w, "índice de entregable inválido", http.StatusBadRequest)
		return
	}
	row := DeliverableForm{Index: idx, Status: "not_started"}
	s.renderFragment(w, "activity_wizard", "deliverable_row", row)
	s.renderFragment(w, "activity_wizard", "deliverable_add_button", DeliverableRowData{NextIndex: idx + 1})
}

func (s *Server) handleActivityCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	wf := parseWizardForm(r, s.store.Sections())
	wf.Mode = "new"
	wf.Title = "Nueva actividad"
	wf.Nav = "activities"

	if !(validateStepA(&wf) && validateStepB(&wf) && validateStepC(&wf) && validateStepD(&wf) && validateStepE(&wf)) {
		s.renderFragment(w, "activity_wizard", "wizard_error", map[string]string{
			"Message": "Los datos de un paso anterior ya no son válidos. Vuelve a empezar el cuestionario.",
		})
		return
	}
	if !validateStepF(&wf) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderFragment(w, "activity_wizard", "step6", wf)
		return
	}

	if _, err := s.store.CreateActivity(wf.toActivity()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Redirect", "/activities")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleActivityUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	wf := parseWizardForm(r, s.store.Sections())
	wf.Mode = "edit"
	wf.ActivityID = id
	wf.Title = "Editar actividad"
	wf.Nav = "activities"

	if !(validateStepA(&wf) && validateStepB(&wf) && validateStepC(&wf) && validateStepD(&wf) && validateStepE(&wf)) {
		s.renderFragment(w, "activity_wizard", "wizard_error", map[string]string{
			"Message": "Los datos de un paso anterior ya no son válidos. Vuelve a empezar la edición.",
		})
		return
	}
	if !validateStepF(&wf) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderFragment(w, "activity_wizard", "step6", wf)
		return
	}

	if _, err := s.store.UpdateActivity(id, wf.toActivity()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Redirect", "/activities")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleActivityDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteActivity(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	snap := s.store.Snapshot()
	s.renderFragment(w, "activities_list", "activities_table", ListPageData{
		Activities: snap.Activities,
	})
}
