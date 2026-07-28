// Package store implementa la persistencia JSON atómica del Store de Project Mapper.
package store

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"projectmapper/internal/model"
)

// Store envuelve model.Store con acceso concurrente seguro y persistencia
// atómica en disco (archivo temporal + rename).
type Store struct {
	mu   sync.RWMutex
	path string
	data model.Store
}

// Load abre el JSON en path. Si no existe, crea un Store con los valores
// default y lo escribe inmediatamente en disco.
func Load(path string) (*Store, error) {
	s := &Store{path: path}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = model.NewStore()
			if err := atomicWrite(path, s.data); err != nil {
				return nil, fmt.Errorf("crear store inicial: %w", err)
			}
			return s, nil
		}
		return nil, fmt.Errorf("leer store: %w", err)
	}

	var d model.Store
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parsear store: %w", err)
	}
	if d.Sections == nil {
		d.Sections = model.DefaultSections()
	}
	if d.Weights.Impact == nil || d.Weights.Effort == nil {
		d.Weights = model.DefaultWeights()
	}
	if d.Activities == nil {
		d.Activities = []model.Activity{}
	}
	s.data = d
	return s, nil
}

// atomicWrite serializa data como JSON indentado y lo escribe de forma
// atómica: archivo temporal en el mismo directorio, luego os.Rename.
func atomicWrite(path string, data model.Store) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "projects-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Save persiste el estado actual en disco.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWrite(s.path, s.data)
}

// Snapshot devuelve una copia profunda del Store para lectura/render.
func (s *Store) Snapshot() model.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return deepCopy(s.data)
}

func deepCopy(d model.Store) model.Store {
	b, _ := json.Marshal(d)
	var cp model.Store
	_ = json.Unmarshal(b, &cp)
	return cp
}

func nextIDLocked(activities []model.Activity) string {
	max := 0
	for _, a := range activities {
		var n int
		if _, err := fmt.Sscanf(a.ID, "ACT-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("ACT-%03d", max+1)
}

// CreateActivity asigna un ID secuencial y timestamps, añade la actividad y
// persiste el store.
func (s *Store) CreateActivity(a model.Activity) (model.Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	a.ID = nextIDLocked(s.data.Activities)
	a.CreatedAt = now
	a.UpdatedAt = now
	s.data.Activities = append(s.data.Activities, a)

	if err := atomicWrite(s.path, s.data); err != nil {
		return model.Activity{}, err
	}
	return a, nil
}

// UpdateActivity reemplaza la actividad con el ID dado, preservando su
// CreatedAt original.
func (s *Store) UpdateActivity(id string, upd model.Activity) (model.Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Activities {
		if s.data.Activities[i].ID == id {
			upd.ID = id
			upd.CreatedAt = s.data.Activities[i].CreatedAt
			upd.UpdatedAt = time.Now().UTC()
			s.data.Activities[i] = upd
			if err := atomicWrite(s.path, s.data); err != nil {
				return model.Activity{}, err
			}
			return upd, nil
		}
	}
	return model.Activity{}, fmt.Errorf("actividad %s no encontrada", id)
}

// DeleteActivity elimina la actividad con el ID dado.
func (s *Store) DeleteActivity(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, a := range s.data.Activities {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("actividad %s no encontrada", id)
	}
	s.data.Activities = append(s.data.Activities[:idx], s.data.Activities[idx+1:]...)
	return atomicWrite(s.path, s.data)
}

// GetActivity busca una actividad por ID.
func (s *Store) GetActivity(id string) (model.Activity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.data.Activities {
		if a.ID == id {
			var cp model.Activity
			b, _ := json.Marshal(a)
			_ = json.Unmarshal(b, &cp)
			return cp, true
		}
	}
	return model.Activity{}, false
}

// Sections devuelve las secciones organizacionales configuradas.
func (s *Store) Sections() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.data.Sections))
	copy(out, s.data.Sections)
	return out
}

// SetSections reemplaza la lista de secciones.
func (s *Store) SetSections(sections []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Sections = sections
	return atomicWrite(s.path, s.data)
}

// Weights devuelve una copia de los pesos actuales.
func (s *Store) Weights() model.Weights {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyWeights(s.data.Weights)
}

func copyWeights(w model.Weights) model.Weights {
	out := model.Weights{
		Impact: make(map[string]float64, len(w.Impact)),
		Effort: make(map[string]float64, len(w.Effort)),
	}
	maps.Copy(out.Impact, w.Impact)
	maps.Copy(out.Effort, w.Effort)
	return out
}

// SetWeights reemplaza los pesos de impacto/esfuerzo.
func (s *Store) SetWeights(w model.Weights) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Weights = w
	return atomicWrite(s.path, s.data)
}

// Thresholds devuelve los umbrales de clasificación actuales.
func (s *Store) Thresholds() model.Thresholds {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Thresholds
}

// SetThresholds reemplaza los umbrales de clasificación.
func (s *Store) SetThresholds(t model.Thresholds) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Thresholds = t
	return atomicWrite(s.path, s.data)
}

// ReplaceAll sobreescribe todo el store (usado por -seed al arrancar).
func (s *Store) ReplaceAll(d model.Store) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = d
	return atomicWrite(s.path, s.data)
}
