// Package server implementa el servidor HTTP de Project Mapper: mux,
// middleware, templates embebidos y handlers.
package server

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"

	"projectmapper/internal/store"
	"projectmapper/web"
)

// Server agrupa el estado del servidor HTTP.
type Server struct {
	store *store.Store
	mux   *http.ServeMux
	tmpl  map[string]*template.Template
}

// New construye el Server, parsea templates y registra rutas.
func New(st *store.Store) *Server {
	s := &Server{
		store: st,
		mux:   http.NewServeMux(),
		tmpl:  map[string]*template.Template{},
	}
	s.loadTemplates()
	s.routes()
	return s
}

// Handler devuelve el http.Handler final (con middleware) listo para servir.
func (s *Server) Handler() http.Handler {
	return loggingMiddleware(s.mux)
}

func (s *Server) mustParse(group string, files ...string) {
	all := append([]string{"templates/layout.html.tmpl"}, files...)
	t := template.Must(template.New("layout").Funcs(funcMap()).ParseFS(web.TemplatesFS, all...))
	s.tmpl[group] = t
}

func (s *Server) loadTemplates() {
	s.mustParse("activities_list", "templates/activities_list.html.tmpl")
	s.mustParse("activity_wizard", "templates/activity_wizard.html.tmpl")
	s.mustParse("config", "templates/config.html.tmpl")
	s.mustParse("matrix_impact_effort",
		"templates/matrix_svg.svg.tmpl", "templates/activity_detail.html.tmpl", "templates/matrix_impact_effort.html.tmpl")
	s.mustParse("matrix_eisenhower",
		"templates/matrix_svg.svg.tmpl", "templates/activity_detail.html.tmpl", "templates/matrix_eisenhower.html.tmpl")
	s.mustParse("org_swimlanes", "templates/swimlane_svg.svg.tmpl", "templates/org_swimlanes.html.tmpl")
	s.mustParse("org_treemap", "templates/treemap_svg.svg.tmpl", "templates/org_treemap.html.tmpl")
	s.mustParse("timeline", "templates/timeline_svg.svg.tmpl", "templates/timeline.html.tmpl")
	s.mustParse("home", "templates/home.html.tmpl")
}

func (s *Server) routes() {
	staticSub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	s.mux.HandleFunc("GET /{$}", s.handleHome)

	s.mux.HandleFunc("GET /activities", s.handleActivitiesList)
	s.mux.HandleFunc("GET /activities/new", s.handleWizardNew)
	s.mux.HandleFunc("GET /activities/wizard/deliverable-row", s.handleDeliverableRow)
	s.mux.HandleFunc("POST /activities/wizard/step2", s.wizardAdvance(validateStepA, "step1", "step2"))
	s.mux.HandleFunc("POST /activities/wizard/step3", s.wizardAdvance(validateStepB, "step2", "step3"))
	s.mux.HandleFunc("POST /activities/wizard/step4", s.wizardAdvance(validateStepC, "step3", "step4"))
	s.mux.HandleFunc("POST /activities/wizard/step5", s.wizardAdvance(validateStepD, "step4", "step5"))
	s.mux.HandleFunc("POST /activities/wizard/step6", s.wizardAdvance(validateStepE, "step5", "step6"))
	s.mux.HandleFunc("POST /activities", s.handleActivityCreate)
	s.mux.HandleFunc("GET /activities/{id}/edit", s.handleWizardEdit)
	s.mux.HandleFunc("PUT /activities/{id}", s.handleActivityUpdate)
	s.mux.HandleFunc("DELETE /activities/{id}", s.handleActivityDelete)

	s.mux.HandleFunc("GET /config", s.handleConfigPage)
	s.mux.HandleFunc("POST /config/sections", s.handleConfigSections)
	s.mux.HandleFunc("POST /config/weights", s.handleConfigWeights)

	s.mux.HandleFunc("GET /matrix/impact-effort", s.handleMatrixImpactEffort)
	s.mux.HandleFunc("GET /matrix/impact-effort/svg", s.handleMatrixImpactEffortSVG)
	s.mux.HandleFunc("GET /matrix/impact-effort/detail/{id}", s.handleMatrixImpactEffortDetail)
	s.mux.HandleFunc("GET /matrix/eisenhower", s.handleMatrixEisenhower)
	s.mux.HandleFunc("GET /matrix/eisenhower/svg", s.handleMatrixEisenhowerSVG)
	s.mux.HandleFunc("GET /matrix/eisenhower/detail/{id}", s.handleMatrixEisenhowerDetail)

	s.mux.HandleFunc("GET /org/swimlanes", s.handleOrgSwimlanes)
	s.mux.HandleFunc("GET /org/treemap", s.handleOrgTreemap)
	s.mux.HandleFunc("GET /timeline", s.handleTimeline)
	s.mux.HandleFunc("GET /timeline/svg", s.handleTimelineSVG)
}

// isHX reporta si la petición viene de htmx (para decidir fragmento vs página completa).
func isHX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) renderFull(w http.ResponseWriter, group string, data any) {
	t, ok := s.tmpl[group]
	if !ok {
		http.Error(w, "template group desconocido: "+group, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render error (%s/layout): %v", group, err)
		http.Error(w, "error de render", http.StatusInternalServerError)
	}
}

func (s *Server) renderFragment(w http.ResponseWriter, group, name string, data any) {
	t, ok := s.tmpl[group]
	if !ok {
		http.Error(w, "template group desconocido: "+group, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render error (%s/%s): %v", group, name, err)
		http.Error(w, "error de render", http.StatusInternalServerError)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}
