package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"projectmapper/internal/model"
)

const maxDeliverables = 30

// activityTypes son los valores válidos de Activity.Type (A2 de la propuesta).
var activityTypes = []string{"Project", "Workstream", "Recurring", "AdHoc"}

// c1Bands son las bandas válidas de esfuerzo en persona-días (C1).
var c1Bands = []string{"<5", "5-20", "20-60", "60-120", ">120"}

var c4Values = []string{"none", "some", "blocking"}
var d2Values = []string{"nothing", "friction", "escalation", "breach"}
var d3Values = []string{"mine", "shared", "other"}
var d4Values = []string{"yes", "partial", "no"}
var statusValues = []string{"not_started", "in_progress", "at_risk", "done"}

// f2Keys son las claves sobre las que se puede marcar "fue una conjetura".
var f2Keys = []string{"B1", "B2", "B3", "B4", "C1", "C2", "C3", "C4", "D1", "D2", "D3", "D4"}

// DeliverableForm es la representación de un entregable dentro del cuestionario.
type DeliverableForm struct {
	Index     int
	Name      string
	Due       string
	Status    string
	DependsOn string
}

// WizardForm acumula el estado del cuestionario multipaso. No hay estado en
// el servidor: cada paso viaja completo como hidden fields (ver plan §3 Fase 1).
type WizardForm struct {
	Title string
	Nav   string

	Mode       string // "new" | "edit"
	ActivityID string
	Sections   []string
	Errors     map[string]string

	Name        string
	Type        string
	Owner       string
	Section     string
	Involved    []string
	Description string

	B1, B2, B3, B4 int

	C1 string
	C2 int
	C3 int
	C4 string

	NoDeadline bool
	D1         string
	D2         string
	D3         string
	D4         string

	Deliverables []DeliverableForm

	F1 int
	F2 []string
	F3 string
}

func atoiDefault(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func parseWizardForm(r *http.Request, sections []string) WizardForm {
	wf := WizardForm{Sections: sections, Errors: map[string]string{}}

	wf.Mode = r.FormValue("mode")
	if wf.Mode == "" {
		wf.Mode = "new"
	}
	wf.ActivityID = r.FormValue("activity_id")

	wf.Name = strings.TrimSpace(r.FormValue("name"))
	wf.Type = r.FormValue("type")
	wf.Owner = strings.TrimSpace(r.FormValue("owner"))
	wf.Section = r.FormValue("section")
	wf.Involved = r.Form["involved"]
	wf.Description = strings.TrimSpace(r.FormValue("description"))

	wf.B1 = atoiDefault(r.FormValue("b1"))
	wf.B2 = atoiDefault(r.FormValue("b2"))
	wf.B3 = atoiDefault(r.FormValue("b3"))
	wf.B4 = atoiDefault(r.FormValue("b4"))

	wf.C1 = r.FormValue("c1")
	wf.C2 = atoiDefault(r.FormValue("c2"))
	wf.C3 = atoiDefault(r.FormValue("c3"))
	wf.C4 = r.FormValue("c4")

	wf.NoDeadline = r.FormValue("no_deadline") == "on"
	wf.D1 = r.FormValue("d1")
	wf.D2 = r.FormValue("d2")
	wf.D3 = r.FormValue("d3")
	wf.D4 = r.FormValue("d4")

	for i := range maxDeliverables {
		prefix := fmt.Sprintf("deliverables[%d]", i)
		name := strings.TrimSpace(r.FormValue(prefix + ".name"))
		if name == "" {
			continue
		}
		wf.Deliverables = append(wf.Deliverables, DeliverableForm{
			Index:     len(wf.Deliverables),
			Name:      name,
			Due:       r.FormValue(prefix + ".due"),
			Status:    r.FormValue(prefix + ".status"),
			DependsOn: strings.TrimSpace(r.FormValue(prefix + ".depends_on")),
		})
	}

	wf.F1 = atoiDefault(r.FormValue("f1"))
	wf.F2 = r.Form["f2"]
	wf.F3 = strings.TrimSpace(r.FormValue("f3"))

	return wf
}

func in1to5(v int) bool { return v >= 1 && v <= 5 }

func oneOf(v string, options []string) bool {
	return slices.Contains(options, v)
}

// validateStepA valida la Sección A (Identificación).
func validateStepA(wf *WizardForm) bool {
	ok := true
	if wf.Name == "" {
		wf.Errors["name"] = "El nombre es obligatorio."
		ok = false
	}
	if !oneOf(wf.Type, activityTypes) {
		wf.Errors["type"] = "Selecciona un tipo válido."
		ok = false
	}
	if wf.Owner == "" {
		wf.Errors["owner"] = "El owner es obligatorio."
		ok = false
	}
	if wf.Section == "" || !contains(wf.Sections, wf.Section) {
		wf.Errors["section"] = "Selecciona una sección válida."
		ok = false
	}
	return ok
}

// validateStepB valida la Sección B (Impacto).
func validateStepB(wf *WizardForm) bool {
	ok := true
	for _, f := range []struct {
		key string
		val int
	}{{"b1", wf.B1}, {"b2", wf.B2}, {"b3", wf.B3}, {"b4", wf.B4}} {
		if !in1to5(f.val) {
			wf.Errors[f.key] = "Selecciona un valor de 1 a 5."
			ok = false
		}
	}
	return ok
}

// validateStepC valida la Sección C (Esfuerzo).
func validateStepC(wf *WizardForm) bool {
	ok := true
	if !oneOf(wf.C1, c1Bands) {
		wf.Errors["c1"] = "Selecciona una banda válida."
		ok = false
	}
	if !in1to5(wf.C2) {
		wf.Errors["c2"] = "Selecciona un valor de 1 a 5."
		ok = false
	}
	if !in1to5(wf.C3) {
		wf.Errors["c3"] = "Selecciona un valor de 1 a 5."
		ok = false
	}
	if !oneOf(wf.C4, c4Values) {
		wf.Errors["c4"] = "Selecciona una opción válida."
		ok = false
	}
	return ok
}

// validateStepD valida la Sección D (Urgencia e Importancia).
func validateStepD(wf *WizardForm) bool {
	ok := true
	if !wf.NoDeadline {
		if wf.D1 == "" {
			wf.Errors["d1"] = "Indica una fecha o marca \"sin deadline\"."
			ok = false
		} else if _, err := time.Parse("2006-01-02", wf.D1); err != nil {
			wf.Errors["d1"] = "Fecha inválida."
			ok = false
		}
	}
	if !oneOf(wf.D2, d2Values) {
		wf.Errors["d2"] = "Selecciona una opción válida."
		ok = false
	}
	if !oneOf(wf.D3, d3Values) {
		wf.Errors["d3"] = "Selecciona una opción válida."
		ok = false
	}
	if !oneOf(wf.D4, d4Values) {
		wf.Errors["d4"] = "Selecciona una opción válida."
		ok = false
	}
	return ok
}

// validateStepE valida la Sección E (Entregables): opcional en conjunto,
// pero cada fila con nombre debe tener fecha y estado válidos.
func validateStepE(wf *WizardForm) bool {
	ok := true
	for i, d := range wf.Deliverables {
		key := fmt.Sprintf("deliverable_%d_due", i)
		if d.Due == "" {
			wf.Errors[key] = fmt.Sprintf("El entregable %q necesita una fecha.", d.Name)
			ok = false
		} else if _, err := time.Parse("2006-01-02", d.Due); err != nil {
			wf.Errors[key] = fmt.Sprintf("Fecha inválida en el entregable %q.", d.Name)
			ok = false
		}
		if !oneOf(d.Status, statusValues) {
			wf.Deliverables[i].Status = "not_started"
		}
	}
	return ok
}

// validateStepF valida la Sección F (Suficiencia de información).
func validateStepF(wf *WizardForm) bool {
	ok := true
	if !in1to5(wf.F1) {
		wf.Errors["f1"] = "Selecciona un valor de 1 a 5."
		ok = false
	}
	for _, k := range wf.F2 {
		if !contains(f2Keys, k) {
			wf.Errors["f2"] = "Selección inválida."
			ok = false
			break
		}
	}
	return ok
}

// toActivity convierte el WizardForm (ya validado) al modelo persistido.
func (wf WizardForm) toActivity() model.Activity {
	a := model.Activity{
		Name:        wf.Name,
		Type:        wf.Type,
		Owner:       wf.Owner,
		Section:     wf.Section,
		Involved:    wf.Involved,
		Description: wf.Description,
		Answers: model.Answers{
			B1: wf.B1,
			B2: wf.B2,
			B3: wf.B3,
			B4: wf.B4,
			C1: wf.C1,
			C2: wf.C2,
			C3: wf.C3,
			C4: wf.C4,
			D2: wf.D2,
			D3: wf.D3,
			D4: wf.D4,
			F1: wf.F1,
			F2: wf.F2,
			F3: wf.F3,
		},
	}
	if !wf.NoDeadline && wf.D1 != "" {
		d1 := wf.D1
		a.Answers.D1 = &d1
	}
	for i, d := range wf.Deliverables {
		var dep *string
		if d.DependsOn != "" {
			dep = &d.DependsOn
		}
		a.Deliverables = append(a.Deliverables, model.Deliverable{
			ID:        fmt.Sprintf("DLV-%03d", i+1),
			Name:      d.Name,
			Due:       d.Due,
			Status:    d.Status,
			DependsOn: dep,
		})
	}
	return a
}

// wizardFormFromActivity precarga el cuestionario con una actividad existente
// (usado por la edición, que reutiliza el mismo flujo multipaso).
func wizardFormFromActivity(a model.Activity, sections []string) WizardForm {
	wf := WizardForm{
		Mode:        "edit",
		ActivityID:  a.ID,
		Sections:    sections,
		Errors:      map[string]string{},
		Name:        a.Name,
		Type:        a.Type,
		Owner:       a.Owner,
		Section:     a.Section,
		Involved:    a.Involved,
		Description: a.Description,
		B1:          a.Answers.B1,
		B2:          a.Answers.B2,
		B3:          a.Answers.B3,
		B4:          a.Answers.B4,
		C1:          a.Answers.C1,
		C2:          a.Answers.C2,
		C3:          a.Answers.C3,
		C4:          a.Answers.C4,
		D2:          a.Answers.D2,
		D3:          a.Answers.D3,
		D4:          a.Answers.D4,
		F1:          a.Answers.F1,
		F2:          a.Answers.F2,
		F3:          a.Answers.F3,
	}
	if a.Answers.D1 == nil {
		wf.NoDeadline = true
	} else {
		wf.D1 = *a.Answers.D1
	}
	for i, d := range a.Deliverables {
		df := DeliverableForm{Index: i, Name: d.Name, Due: d.Due, Status: d.Status}
		if d.DependsOn != nil {
			df.DependsOn = *d.DependsOn
		}
		wf.Deliverables = append(wf.Deliverables, df)
	}
	return wf
}
