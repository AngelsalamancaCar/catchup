package scoring

import (
	"math"
	"testing"
	"time"

	"projectmapper/internal/model"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func defaultWeights() model.Weights { return model.DefaultWeights() }

func TestNormScale5Table(t *testing.T) {
	cases := []struct {
		v    int
		want float64
	}{
		{1, 0}, {2, 25}, {3, 50}, {4, 75}, {5, 100},
		{0, 0},    // fuera de rango por abajo -> capa a 1
		{6, 100},  // fuera de rango por arriba -> capa a 5
		{-3, 0},   // muy fuera de rango
		{99, 100}, // muy fuera de rango
	}
	for _, c := range cases {
		got := normScale5(c.v)
		if !almostEqual(got, c.want) {
			t.Errorf("normScale5(%d) = %v, esperaba %v", c.v, got, c.want)
		}
	}
}

func TestNormC1Table(t *testing.T) {
	cases := map[string]float64{
		"<5": 10, "5-20": 30, "20-60": 55, "60-120": 80, ">120": 100,
		"":        0,
		"invalid": 0,
	}
	for band, want := range cases {
		if got := normC1(band); !almostEqual(got, want) {
			t.Errorf("normC1(%q) = %v, esperaba %v", band, got, want)
		}
	}
}

func TestNormC4Table(t *testing.T) {
	cases := map[string]float64{"none": 0, "some": 50, "blocking": 100, "": 0}
	for v, want := range cases {
		if got := normC4(v); !almostEqual(got, want) {
			t.Errorf("normC4(%q) = %v, esperaba %v", v, got, want)
		}
	}
}

func TestNormD2Table(t *testing.T) {
	cases := map[string]float64{"nothing": 0, "friction": 33, "escalation": 66, "breach": 100, "": 0}
	for v, want := range cases {
		if got := normD2(v); !almostEqual(got, want) {
			t.Errorf("normD2(%q) = %v, esperaba %v", v, got, want)
		}
	}
}

func TestNormD3Table(t *testing.T) {
	cases := map[string]float64{"mine": 100, "shared": 60, "other": 20, "": 0}
	for v, want := range cases {
		if got := normD3(v); !almostEqual(got, want) {
			t.Errorf("normD3(%q) = %v, esperaba %v", v, got, want)
		}
	}
}

func TestImpactScoreDefaultWeights(t *testing.T) {
	a := model.Answers{B1: 5, B2: 3, B3: 1, B4: 1}
	res := ImpactScore(a, defaultWeights())
	// norm: B1=100, B2=50, B3=0, B4=0 ; pesos 0.4/0.2/0.25/0.15
	want := 0.4*100 + 0.2*50 + 0.25*0 + 0.15*0
	if !almostEqual(res.Score, want) {
		t.Fatalf("ImpactScore = %v, esperaba %v", res.Score, want)
	}
	if len(res.Breakdown) != 4 {
		t.Fatalf("esperaba 4 componentes en el desglose, tengo %d", len(res.Breakdown))
	}
	for _, c := range res.Breakdown {
		if !almostEqual(c.Contribution, c.Weight*c.Normalized) {
			t.Errorf("componente %s: contribution %v != weight*normalized %v", c.Key, c.Contribution, c.Weight*c.Normalized)
		}
	}
}

func TestImpactScoreCustomWeights(t *testing.T) {
	a := model.Answers{B1: 5, B2: 5, B3: 5, B4: 5}
	w := model.Weights{Impact: map[string]float64{"B1": 0.25, "B2": 0.25, "B3": 0.25, "B4": 0.25}}
	res := ImpactScore(a, w)
	if !almostEqual(res.Score, 100) {
		t.Fatalf("ImpactScore con B1-4=5 y pesos iguales = %v, esperaba 100", res.Score)
	}
}

func TestEffortScoreBands(t *testing.T) {
	cases := []struct {
		c1   string
		want float64
	}{
		{"<5", 10}, {"5-20", 30}, {"20-60", 55}, {"60-120", 80}, {">120", 100},
	}
	w := defaultWeights()
	for _, c := range cases {
		a := model.Answers{C1: c.c1, C2: 1, C3: 1, C4: "none"}
		res := EffortScore(a, w)
		want := 0.4 * c.want // resto de componentes normalizan a 0
		if !almostEqual(res.Score, want) {
			t.Errorf("EffortScore(C1=%s) = %v, esperaba %v", c.c1, res.Score, want)
		}
	}
}

func TestEffortScoreC2CappedAt5(t *testing.T) {
	w := defaultWeights()
	a1 := model.Answers{C1: "<5", C2: 5, C3: 1, C4: "none"}
	a2 := model.Answers{C1: "<5", C2: 12, C3: 1, C4: "none"}
	r1 := EffortScore(a1, w)
	r2 := EffortScore(a2, w)
	if !almostEqual(r1.Score, r2.Score) {
		t.Fatalf("C2=5 y C2=12 deberían normalizar igual (capado a 5): %v vs %v", r1.Score, r2.Score)
	}
}

func TestUrgencyScoreDeadlineOverdue(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	d1 := "2026-07-20" // vencido hace 7 días
	a := model.Answers{D1: &d1, D2: "nothing"}
	res := UrgencyScore(a, now)
	want := 0.6 * 100 // proximidad vencido = 100, D2 nothing = 0
	if !almostEqual(res.Score, want) {
		t.Fatalf("UrgencyScore vencido = %v, esperaba %v", res.Score, want)
	}
}

func TestUrgencyScoreDeadlineToday(t *testing.T) {
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	d1 := "2026-07-27"
	a := model.Answers{D1: &d1, D2: "nothing"}
	res := UrgencyScore(a, now)
	want := 0.6 * 100
	if !almostEqual(res.Score, want) {
		t.Fatalf("UrgencyScore hoy = %v, esperaba %v", res.Score, want)
	}
}

func TestUrgencyScoreDeadline60Days(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d1 := now.AddDate(0, 0, 60).Format("2006-01-02")
	a := model.Answers{D1: &d1, D2: "nothing"}
	res := UrgencyScore(a, now)
	if !almostEqual(res.Score, 0) {
		t.Fatalf("UrgencyScore a 60 días = %v, esperaba 0 (rampa agotada)", res.Score)
	}
}

func TestUrgencyScoreDeadlineBeyond60Days(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d1 := now.AddDate(0, 0, 120).Format("2006-01-02")
	a := model.Answers{D1: &d1, D2: "nothing"}
	res := UrgencyScore(a, now)
	if !almostEqual(res.Score, 0) {
		t.Fatalf("UrgencyScore a 120 días = %v, esperaba 0 (capado, no negativo)", res.Score)
	}
}

func TestUrgencyScoreDeadline30Days(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d1 := now.AddDate(0, 0, 30).Format("2006-01-02")
	a := model.Answers{D1: &d1, D2: "nothing"}
	res := UrgencyScore(a, now)
	wantProx := 50.0 // 100 - 30*(100/60)
	want := 0.6 * wantProx
	if !almostEqual(res.Score, want) {
		t.Fatalf("UrgencyScore a 30 días = %v, esperaba %v", res.Score, want)
	}
}

func TestUrgencyScoreNoDeadline(t *testing.T) {
	now := time.Now()
	a := model.Answers{D1: nil, D2: "breach"}
	res := UrgencyScore(a, now)
	want := 0.4 * 100 // proximidad=0, D2 breach=100
	if !almostEqual(res.Score, want) {
		t.Fatalf("UrgencyScore sin deadline = %v, esperaba %v", res.Score, want)
	}
}

func TestUrgencyScoreInvalidDeadlineTreatedAsNone(t *testing.T) {
	now := time.Now()
	bad := "no-es-una-fecha"
	a := model.Answers{D1: &bad, D2: "nothing"}
	res := UrgencyScore(a, now)
	if !almostEqual(res.Score, 0) {
		t.Fatalf("UrgencyScore con fecha inválida = %v, esperaba 0", res.Score)
	}
}

func TestImportanceScore(t *testing.T) {
	a := model.Answers{B1: 5, D3: "shared"}
	res := ImportanceScore(a)
	want := 0.6*100 + 0.4*60
	if !almostEqual(res.Score, want) {
		t.Fatalf("ImportanceScore = %v, esperaba %v", res.Score, want)
	}
}

func TestUncertaintyScoreConfidenceOnly(t *testing.T) {
	cases := []struct {
		f1   int
		want float64
	}{
		{1, 100}, {2, 75}, {3, 50}, {4, 25}, {5, 0},
	}
	for _, c := range cases {
		a := model.Answers{F1: c.f1}
		res := UncertaintyScore(a)
		if !almostEqual(res.Score, c.want) {
			t.Errorf("UncertaintyScore(F1=%d) = %v, esperaba %v", c.f1, res.Score, c.want)
		}
	}
}

func TestUncertaintyScoreGuessBonus(t *testing.T) {
	a := model.Answers{F1: 5, F2: []string{"B1", "C1"}} // base 0 + 2*10 = 20
	res := UncertaintyScore(a)
	if !almostEqual(res.Score, 20) {
		t.Fatalf("UncertaintyScore con 2 conjeturas = %v, esperaba 20", res.Score)
	}
}

func TestUncertaintyScoreCapAt100(t *testing.T) {
	a := model.Answers{F1: 1, F2: []string{"B1", "B2", "B3", "B4", "C1", "C2", "C3", "C4", "D1", "D2", "D3", "D4"}}
	res := UncertaintyScore(a)
	if res.Score != 100 {
		t.Fatalf("UncertaintyScore con base 100 + 12 conjeturas = %v, esperaba capado a 100", res.Score)
	}
}

func TestUncertaintyScoreF1Equals2ShowsHighUncertainty(t *testing.T) {
	// Caso del criterio de aceptación: F1=2 debe producir borde punteado
	// (NeedsInfo) porque Uncertainty=75 >= 50.
	a := model.Answers{F1: 2}
	res := UncertaintyScore(a)
	if res.Score < 50 {
		t.Fatalf("UncertaintyScore(F1=2) = %v, esperaba >= 50 para disparar el overlay de info", res.Score)
	}
}

func TestInfoPriorityScore(t *testing.T) {
	got := InfoPriorityScore(80, 50)
	want := 40.0
	if !almostEqual(got, want) {
		t.Fatalf("InfoPriorityScore(80,50) = %v, esperaba %v", got, want)
	}
}

func TestClassifyImpactEffortQuadrants(t *testing.T) {
	th := model.DefaultThresholds() // 50/50
	cases := []struct {
		impact, effort float64
		want           string
	}{
		{80, 20, QuadrantQuickWins},
		{80, 80, QuadrantMajorProjects},
		{20, 20, QuadrantFillIns},
		{20, 80, QuadrantThanklessTasks},
		{50, 50, QuadrantMajorProjects}, // en el umbral: >= cuenta como "alto"
	}
	for _, c := range cases {
		got := ClassifyImpactEffort(c.impact, c.effort, th)
		if got != c.want {
			t.Errorf("ClassifyImpactEffort(%v,%v) = %s, esperaba %s", c.impact, c.effort, got, c.want)
		}
	}
}

func TestClassifyEisenhowerQuadrants(t *testing.T) {
	th := model.DefaultThresholds()
	cases := []struct {
		urgency, importance float64
		want                string
	}{
		{80, 80, EisenhowerDo},
		{20, 80, EisenhowerSchedule},
		{80, 20, EisenhowerDelegate},
		{20, 20, EisenhowerDelete},
	}
	for _, c := range cases {
		got := ClassifyEisenhower(c.urgency, c.importance, th)
		if got != c.want {
			t.Errorf("ClassifyEisenhower(%v,%v) = %s, esperaba %s", c.urgency, c.importance, got, c.want)
		}
	}
}

func TestBorderlineBand(t *testing.T) {
	cases := []struct {
		score, threshold float64
		want             bool
	}{
		{50, 50, true},
		{58, 50, true}, // exactamente +8
		{42, 50, true}, // exactamente -8
		{58.1, 50, false},
		{41.9, 50, false},
		{100, 50, false},
	}
	for _, c := range cases {
		got := Borderline(c.score, c.threshold)
		if got != c.want {
			t.Errorf("Borderline(%v,%v) = %v, esperaba %v", c.score, c.threshold, got, c.want)
		}
	}
}

func TestComputeIntegration(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	d1 := "2026-07-27"
	a := model.Activity{
		ID: "ACT-001",
		Answers: model.Answers{
			B1: 5, B2: 4, B3: 5, B4: 4,
			C1: ">120", C2: 4, C3: 5, C4: "blocking",
			D1: &d1, D2: "escalation", D3: "shared", D4: "no",
			F1: 2, F2: []string{"C1"},
		},
	}
	scores := Compute(a, model.DefaultWeights(), model.DefaultThresholds(), now)

	if scores.ActivityID != "ACT-001" {
		t.Fatalf("ActivityID = %s", scores.ActivityID)
	}
	if scores.ImpactEffortQuadrant != QuadrantMajorProjects {
		t.Fatalf("ImpactEffortQuadrant = %s, esperaba %s (impact y effort altos)", scores.ImpactEffortQuadrant, QuadrantMajorProjects)
	}
	if scores.EisenhowerQuadrant != EisenhowerDo {
		t.Fatalf("EisenhowerQuadrant = %s, esperaba %s (deadline hoy + importante)", scores.EisenhowerQuadrant, EisenhowerDo)
	}
	if !scores.NeedsInfo {
		t.Fatalf("NeedsInfo = false, esperaba true (F1=2 y F2 no vacío)")
	}
	if scores.DecideWithWhatYouHave {
		t.Fatalf("DecideWithWhatYouHave = true, no esperado con alta importancia/incertidumbre")
	}
}

func TestComputeDecideWithWhatYouHave(t *testing.T) {
	now := time.Now()
	a := model.Activity{
		Answers: model.Answers{
			B1: 1, B2: 1, B3: 1, B4: 1,
			C1: "<5", C2: 1, C3: 1, C4: "none",
			D1: nil, D2: "nothing", D3: "other", D4: "yes",
			F1: 5, F2: nil,
		},
	}
	scores := Compute(a, model.DefaultWeights(), model.DefaultThresholds(), now)
	if !scores.DecideWithWhatYouHave {
		t.Fatalf("esperaba DecideWithWhatYouHave=true con baja importancia y baja incertidumbre")
	}
	if scores.NeedsInfo {
		t.Fatalf("esperaba NeedsInfo=false con F1=5 y F2 vacío")
	}
}

func TestImpactEffortLabels(t *testing.T) {
	cases := map[string]string{
		QuadrantQuickWins:      "Quick Wins",
		QuadrantMajorProjects:  "Major Projects",
		QuadrantFillIns:        "Fill-Ins",
		QuadrantThanklessTasks: "Thankless Tasks",
		"unknown":              "unknown",
	}
	for slug, want := range cases {
		if got := ImpactEffortLabel(slug); got != want {
			t.Errorf("ImpactEffortLabel(%s) = %s, esperaba %s", slug, got, want)
		}
	}
}

func TestEisenhowerLabels(t *testing.T) {
	cases := map[string]string{
		EisenhowerDo:       "Hacer",
		EisenhowerSchedule: "Agendar",
		EisenhowerDelegate: "Delegar",
		EisenhowerDelete:   "Eliminar",
		"unknown":          "unknown",
	}
	for slug, want := range cases {
		if got := EisenhowerLabel(slug); got != want {
			t.Errorf("EisenhowerLabel(%s) = %s, esperaba %s", slug, got, want)
		}
	}
}
