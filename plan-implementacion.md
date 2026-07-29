# Plan de Implementación — Project Mapper

> **Uso de este documento:** es un playbook para un agente de Claude Code. Ejecuta las fases en orden. Cada fase tiene tareas concretas, archivos a crear y criterios de aceptación verificables. No avances de fase hasta que los criterios de la fase actual pasen. La fuente de requisitos es `proposal-project-mapper.md`; ante ambigüedad, ese documento manda.

## Estado de avance

| Fase | Estado | Commit |
|---|---|---|
| 1 — Esqueleto, modelo, persistencia, cuestionario | ✅ Completa | `8e91312` |
| 2 — Motor de scoring + matrices SVG | ✅ Completa | `dbdb633` |
| 3 — Mapa organizacional + timeline | ✅ Completa | `feat(fase-3)`, ver `git log` |
| 4 — Export + empaquetado + script Python | ⬜ Pendiente | — |
| 5 — Piloto y calibración | ⬜ Pendiente | — |

Detalle de qué se implementó y qué decisiones no explicitadas por el plan se tomaron en cada fase completada: ver "Notas de implementación" al final de cada fase en la §3. Guía de arquitectura para retomar el trabajo: `CLAUDE.md`.

---

## 0. Instrucciones de operación para el agente

1. **Trabaja fase por fase.** Al terminar cada fase: `go build ./...`, `go vet ./...`, `go test ./...` deben pasar sin errores.
2. **Commits:** un commit por fase como mínimo (si el repo tiene git; si no, inicialízalo en la Fase 1). Mensaje: `feat(fase-N): <resumen>`.
3. **Stack cerrado:** Go (stdlib + `github.com/xuri/excelize/v2` únicamente), HTMX vendorizado, Python solo para el script opcional de importación (Fase 4). **Prohibido:** Node, frameworks JS, routers externos, ORMs, CGO.
4. **Verificación manual:** tras cada fase que toque UI, arranca el binario (`go run ./cmd/projectmapper`) y verifica con `curl http://127.0.0.1:8787/<ruta>` que las rutas responden 200 y contienen los elementos clave indicados en los criterios.
5. **Decisiones ya tomadas (no reabrir):**
   - Persistencia: **JSON único** (`data/projects.json`) con escritura atómica (temp + rename) y mutex. SQLite queda descartado para v1.
   - Router: `net/http` de Go 1.22+ con patrones de método (`mux.HandleFunc("GET /activities/{id}", ...)`).
   - SVG: generado server-side con `html/template` (plantillas `.svg.tmpl`), nunca con JS.
   - Puerto: `127.0.0.1:8787`, configurable con flag `-port`.
   - Idioma de la UI: **español**. Identificadores de código: inglés.
6. **Todo score computado debe ser trazable:** la función de scoring devuelve, además del número, el desglose (respuesta × peso por dimensión). La UI lo muestra en el detalle de cada actividad (requisito de auditabilidad, §7 de la propuesta).

---

## 1. Estructura del repositorio (crear en Fase 1)

```
catchup/
├── cmd/projectmapper/main.go        # flags, arranque servidor, apertura de navegador
├── internal/
│   ├── model/model.go               # structs: Store, Activity, Deliverable, Weights, Answers
│   ├── store/store.go               # carga/guardado JSON atómico, mutex, IDs (ACT-NNN)
│   ├── scoring/scoring.go           # motor de puntuación (puro, sin I/O)
│   ├── scoring/scoring_test.go
│   ├── server/server.go             # mux, middleware, static
│   ├── server/handlers_activity.go  # CRUD + cuestionario multipaso
│   ├── server/handlers_matrix.go    # impacto/esfuerzo + eisenhower
│   ├── server/handlers_org.go       # swimlanes + treemap
│   ├── server/handlers_timeline.go  # gantt
│   ├── server/handlers_export.go    # xlsx, svg, backup json
│   └── svg/                         # helpers de geometría (escalas, posiciones)
├── web/
│   ├── static/htmx.min.js           # vendorizado (v2.x, descargar una vez)
│   ├── static/app.css               # CSS propio, sin frameworks
│   └── templates/                   # *.html.tmpl y *.svg.tmpl (embed.FS)
├── scripts/import_backlog.py        # Fase 4, opcional
├── data/                            # projects.json (gitignored), seed de ejemplo
├── export/                          # salidas xlsx/svg/png (gitignored)
└── go.mod                           # module projectmapper
```

---

## 2. Modelo de datos y reglas de scoring (referencia normativa)

### 2.1 Structs Go (Fase 1)

```go
type Store struct {
    Sections   []string            `json:"sections"`
    Weights    Weights             `json:"weights"`
    Thresholds Thresholds          `json:"thresholds"` // impact/effort/urgency/importance, default 50
    Activities []Activity          `json:"activities"`
}

type Weights struct {
    Impact map[string]float64 `json:"impact"` // claves B1..B4
    Effort map[string]float64 `json:"effort"` // claves C1..C4
}

type Activity struct {
    ID          string        `json:"id"`   // ACT-001, secuencial
    Name        string        `json:"name"`
    Type        string        `json:"type"` // Project|Workstream|Recurring|AdHoc
    Owner       string        `json:"owner"`
    Section     string        `json:"section"`
    Involved    []string      `json:"involved"`
    Description string        `json:"description"`
    Answers     Answers       `json:"answers"`
    Deliverables []Deliverable `json:"deliverables"`
    CreatedAt   time.Time     `json:"created_at"`
    UpdatedAt   time.Time     `json:"updated_at"`
}

type Answers struct {
    B1, B2, B3, B4 int      `json:"..."` // 1–5
    C1             string   `json:"c1"`  // banda: "<5"|"5-20"|"20-60"|"60-120"|">120"
    C2             int      `json:"c2"`  // nº de secciones que coordinan
    C3             int      `json:"c3"`  // 1–5
    C4             string   `json:"c4"`  // "none"|"some"|"blocking"
    D1             *string  `json:"d1"`  // fecha ISO o null (sin deadline)
    D2             string   `json:"d2"`  // "nothing"|"friction"|"escalation"|"breach"
    D3             string   `json:"d3"`  // "mine"|"shared"|"other"
    D4             string   `json:"d4"`  // "yes"|"partial"|"no"
    F1             int      `json:"f1"`  // 1–5 confianza
    F2             []string `json:"f2"`  // claves marcadas como conjetura, ej ["B3","C1"]
    F3             string   `json:"f3"`  // texto libre: info faltante y quién la tiene
}

type Deliverable struct {
    ID        string  `json:"id"`
    Name      string  `json:"name"`
    Due       string  `json:"due"`    // ISO date
    Status    string  `json:"status"` // not_started|in_progress|at_risk|done
    DependsOn *string `json:"depends_on"` // ID de otro deliverable
}
```

**Los scores NO se persisten:** se computan siempre desde `Answers` + `Weights` (fuente única de verdad; evita datos rancios al cambiar pesos).

### 2.2 Normalización a 0–100 (implementar exactamente así, con tests)

| Respuesta | Normalización |
|---|---|
| Escalas 1–5 (B1–B4, C2 capado a 5, C3) | `(v-1)/4 × 100` |
| C1 bandas | `<5`→10, `5-20`→30, `20-60`→55, `60-120`→80, `>120`→100 |
| C4 | none→0, some→50, blocking→100 |
| D2 | nothing→0, friction→33, escalation→66, breach→100 |
| D3 | mine→100, shared→60, other→20 |

### 2.3 Fórmulas de scores

- **Impact** = Σ (peso_Bi × norm(Bi)), pesos default `B1:0.4, B2:0.2, B3:0.25, B4:0.15`.
- **Effort** = Σ (peso_Ci × norm(Ci)), pesos default `C1:0.4, C2:0.2, C3:0.25, C4:0.15`.
- **Urgency** = `0.6 × proximidad(D1) + 0.4 × norm(D2)` donde `proximidad`: sin deadline→0; vencido→100; si no, `max(0, 100 − díasRestantes×(100/60))` (rampa lineal, 60+ días→0, hoy→100).
- **Importance** = `0.6 × norm(B1) + 0.4 × norm(D3)`.
- **Uncertainty** = `(5 − F1)/4 × 100`, +10 por cada clave en F2 (cap 100).
- **InfoPriority** = `Importance × Uncertainty / 100` (rankea el panel "necesita información").

### 2.4 Reglas de clasificación

- **Impacto/Esfuerzo:** umbral default 50/50 ajustable. Cuadrantes: Quick Wins (I alto, E bajo), Major Projects (alto/alto), Fill-Ins (bajo/bajo), Thankless Tasks (bajo/alto).
- **Eisenhower:** Do / Schedule / Delegate / Delete según urgency/importance vs umbrales. Banda fronteriza: items con |score − umbral| ≤ 8 en cualquiera de los dos ejes se listan como "en la frontera".
- **Overlay de información:** `Uncertainty ≥ 50` o `F2` no vacío → borde punteado + badge "ℹ necesita info". `Uncertainty < 25` y `Importance < 40` → etiqueta "decide con lo que tienes".
- **Delegación:** si cuadrante = Delegate y `D4 ∈ {yes, partial}` → sugerir delegación; listar owners de otras actividades de la misma sección como candidatos.
- **Diagnóstico organizacional:** sección con >3 conectores cross-sección → "riesgo de coordinación"; sección con ≥2 Thankless Tasks → "candidata a revisión"; actividad sin sección → "huérfana".
- **At-risk timeline:** `status ≠ done && due < hoy+14d`, o dependencia en riesgo (propagar transitivo, detectar ciclos y romperlos sin colgarse).

---

## 3. Fases de implementación

### FASE 1 — Esqueleto, modelo, persistencia, cuestionario

**Tareas:**
1. `git init` (si aplica), `go mod init projectmapper`, `.gitignore` (data/projects.json, export/, *.exe).
2. Implementar `internal/model` y `internal/store`:
   - `Load(path)`: si no existe el archivo, crear Store con secciones default (`Operaciones, Finanzas, Legal, IT`) y pesos default.
   - `Save()`: marshal indentado → archivo temporal en el mismo dir → `os.Rename`. Mutex RW en todas las operaciones.
   - `NextID()`: ACT-001, ACT-002…
3. Servidor base: mux, middleware de logging, `embed.FS` para templates y static. Layout base HTML con htmx.min.js y app.css. Nav: Actividades | Matriz I/E | Eisenhower | Organización | Timeline | Configuración.
4. Descargar htmx.min.js v2 a `web/static/` (única descarga permitida; si no hay red, pedir al usuario el archivo).
5. **Cuestionario multipaso HTMX** (núcleo de la fase): 6 pasos = secciones A–F de la propuesta (§3).
   - Cada paso es un fragmento (`hx-post` al siguiente paso, `hx-target` sobre el contenedor). Estado parcial en sesión servidor (map en memoria con token) o campos hidden; elegir hidden fields (más simple, sin estado servidor).
   - Escalas 1–5 como radios **con descripciones ancla por punto** (mitigación de riesgo §8; redactar anclas razonables por pregunta).
   - Sección E: grupo repetible de deliverables (botón "+ deliverable" vía `hx-get` que añade fila).
   - Sección F: F2 son checkboxes generados sobre B1–D4.
   - Validación server-side: nombre obligatorio, valores en rango; errores inline sin perder lo tecleado.
6. Listado de actividades (tabla: nombre, tipo, sección, owner, scores computados en caliente, editar/borrar). Edición reutiliza el cuestionario precargado.
7. Página Configuración: editar secciones (lista) y pesos (deben sumar 1.0 por dimensión; validar).
8. `main.go`: flags `-port` (8787) y `-data` (ruta JSON); tras arrancar, abrir navegador (`rundll32 url.dll,FileProtocolHandler` en Windows, `open`/`xdg-open` en mac/linux); mensaje consola: "Project Mapper corriendo en http://127.0.0.1:8787 — cierra esta ventana para detenerlo."
9. Seed: `data/seed.json` con 6–8 actividades ficticias variadas (distintos cuadrantes, con y sin deadline, con F2 marcados) para desarrollo y demo. Flag `-seed` lo copia como data inicial.

**Criterios de aceptación:**
- [x] `go test ./...` pasa; tests de store: roundtrip load/save, escritura atómica, IDs secuenciales.
- [x] Crear actividad completa vía cuestionario de 6 pasos → aparece en el listado → sobrevive a reinicio del binario.
- [x] Editar y borrar funcionan; borrar pide confirmación (`hx-confirm`).
- [x] Bind solo a `127.0.0.1` (verificar que `0.0.0.0` no responde).
- [x] Validación de pesos rechaza sumas ≠ 1.0.

**Notas de implementación (decisiones no explicitadas por el plan):**
- Cuestionario 100% stateless con hidden fields (opción que el plan dejaba abierta en §3 Fase 1 tarea 5); cada paso reenvía todos los campos previos como `<input type="hidden">` y el handler revalida solo el paso recién enviado.
- Grupo repetible de deliverables (sección E) implementado con `hx-get` + out-of-band swap (`hx-swap-oob`) del botón "+ Deliverable" para mantener el índice siguiente sincronizado sin sesión de servidor; el parseo server-side escanea `deliverables[i].name` hasta un máximo (`maxDeliverables`) en vez de confiar en un contador enviado por el cliente.
- Cada "grupo de página" (wizard, listado, config, placeholders) se parsea como su propio `*template.Template` combinando `layout.html.tmpl` + archivo(s) de esa página, porque `html/template` comparte el namespace de `{{define "content"}}` dentro de un mismo set parseado — páginas distintas no pueden compartir grupo de parseo.
- Vistas de Matriz I/E, Eisenhower, Organización y Timeline quedaron como placeholders con mensaje "Disponible en la Fase N" hasta que se implementaran (Matriz I/E y Eisenhower ya se reemplazaron en Fase 2).
- Se agregó `CLAUDE.md` documentando la arquitectura para retomar el trabajo entre sesiones.

### FASE 2 — Motor de scoring + dos matrices SVG

**Tareas:**
1. `internal/scoring`: funciones puras `ImpactScore`, `EffortScore`, `UrgencyScore(now)`, `ImportanceScore`, `Uncertainty`, `Classify*` según §2.2–2.4 de este plan. Cada una devuelve `(score float64, breakdown []Component)`.
2. **Tests exhaustivos** (es el corazón del producto): tabla de casos por normalización, pesos custom, deadline vencido/hoy/60d/null, bordes de banda fronteriza, cap de uncertainty. Cobertura del paquete ≥ 90%.
3. **Matriz Impacto/Esfuerzo** (`GET /matrix/impact-effort`):
   - SVG scatter server-side: ejes 0–100, líneas de cuadrante en los umbrales, etiquetas de cuadrante (Quick Wins, etc.).
   - Punto por actividad: radio según banda C1, color según sección (paleta fija de 8 colores accesibles), `<title>` nativo SVG + panel de detalle vía `hx-get` al hacer click (muestra riesgo D2, dependencias C4/E4, fit estratégico B1 y desglose de scores).
   - Sliders de umbral (2 inputs range) con `hx-trigger="change"` → re-render del fragmento SVG. Persisten en Store.Thresholds.
   - Modo de color alternativo "fit estratégico" (colorear por B1, escala secuencial): toggle HTMX.
4. **Matriz Eisenhower** (`GET /matrix/eisenhower`):
   - Mismo motor de render SVG (extraer helper común en `internal/svg`).
   - Etiquetas de acción: Hacer / Agendar / Delegar / Eliminar.
   - Overlay de información: borde punteado + badge según regla §2.4.
   - Panel lateral: lista rankeada por InfoPriority con F3 visible ("qué falta y quién lo tiene"); sección aparte "decide con lo que tienes"; sección "en la frontera" con la banda ±8.
   - En cuadrante Delegar: sugerencia de delegados según regla §2.4.
5. Ruteo por tipo (A2, §4.3 de la propuesta): Projects/Workstreams destacan en I/E; Recurring/Ad-hoc destacan en Eisenhower. Implementar como filtro default por vista (toggle "mostrar todos").

**Criterios de aceptación:**
- [x] Tests de scoring pasan con cobertura ≥ 90% del paquete. (99.1% `internal/scoring`, 100% `internal/svg`)
- [x] Ambas matrices renderizan el seed correctamente (verificar posiciones de 2 actividades a mano contra las fórmulas). (ACT-001: Impacto 91.25/Esfuerzo 95; ACT-002: Impacto 40/Esfuerzo 18.25, verificado por curl contra cálculo manual)
- [x] Mover slider de umbral reclasifica sin recargar página completa y persiste tras reinicio.
- [x] Actividad con F1=2 muestra borde punteado y aparece en el panel rankeado.
- [x] Click en punto abre detalle con desglose peso×respuesta (auditabilidad).

**Notas de implementación (decisiones no explicitadas por el plan):**
- `scoring.Compute()` es el único punto de entrada que usan los handlers: agrupa los 5 scores, ambas clasificaciones de cuadrante, ambos flags de "en la frontera" y los flags de overlay de información (`NeedsInfo`, `DecideWithWhatYouHave`) en una sola llamada.
- `internal/svg.BuildMatrix()` es el "helper común" pedido por la tarea 4.2: hace toda la aritmética (proyección score→píxel, posición de líneas de umbral, anclas de etiquetas de cuadrante, offset de badges) en Go, así `matrix_svg.svg.tmpl` solo interpola valores ya calculados y nunca hace matemática — evita que el render SVG "se vuelva inmanejable" (riesgo §5 del plan).
- El overlay de información (borde punteado + badge "ℹ") se implementó solo en Eisenhower, no en Impacto/Esfuerzo — el plan lo describe únicamente bajo §4.2/tarea 4, y la matriz I/E usa el click-detail para exponer riesgo/dependencias/fit en su lugar.
- El ruteo por tipo (A2, §4.3) se aplicó en los handlers (`isImpactEffortType`/`isEisenhowerType` en `handlers_matrix.go`), no en `scoring` ni `svg`, para mantener esos paquetes agnósticos del tipo de actividad.
- Sugerencia de delegados verificada por revisión de código (`delegateCandidates`); el seed no contiene un caso con cuadrante Delegar + múltiples owners distintos en la misma sección, así que no se pudo demostrar con datos reales una lista no vacía.

### FASE 3 — Mapa organizacional + timeline

**Tareas:**
1. **Swimlanes** (`GET /org/swimlanes`): SVG con una lane horizontal por sección; tarjetas de actividad en su lane (nombre + cuadrante I/E como chip de color); conectores curvos (path bezier) hacia lanes de A5. Anotaciones automáticas de diagnóstico (§2.4) como badges en la cabecera de lane; actividades huérfanas en lane "Sin asignar".
2. **Treemap** (`GET /org/treemap`): algoritmo slice-and-dice simple (suficiente para <20 secciones; no implementar squarified salvo que quede feo con el seed). Área = person-days (punto medio de banda C1: 2.5/12.5/40/90/150). Click en sección → filtra el swimlane.
3. **Timeline Gantt** (`GET /timeline`): SVG una fila por deliverable agrupado por actividad; escala temporal auto (min due − 7d, max due + 14d); línea de "hoy"; barras coloreadas por status; flechas de dependencia (E4); lógica at-risk con propagación transitiva y detección de ciclos (test obligatorio del ciclo A→B→A).
4. Filtros HTMX del timeline: por sección, por cuadrante (multiselect: Do, Quick Wins, …), por ventana de fechas. Combinables; estado en query params (URLs compartibles).
5. Dashboard home (`GET /`): 4 tarjetas resumen (nº actividades por cuadrante Eisenhower, deliverables at-risk, actividades que necesitan info, secciones con riesgo de coordinación) enlazando a cada vista.

**Criterios de aceptación:**
- [x] Swimlanes muestran conectores cross-sección del seed y al menos una anotación de diagnóstico. (Finanzas: 5 conectores >3 → "riesgo de coordinación", verificado por curl contra `/org/swimlanes`)
- [x] Treemap: áreas proporcionales a person-days (verificar 2 secciones a mano). (Finanzas 240pd → 483.18px, IT 2.5pd → 5.03px de 760px total con 377.5pd totales; `760×240/377.5=483.18` y `760×2.5/377.5=5.03`, verificado por curl contra cálculo manual)
- [x] Gantt: dependencia at-risk propaga al dependiente; ciclo en test no cuelga. (`TestResolveAtRiskPropagatesTransitively`, `TestResolveAtRiskCycleDoesNotHang` con timeout de 2s en goroutine)
- [x] Filtros combinados funcionan y la URL resultante es recargable. (query params `section`/`quadrant`/`from`/`to` combinables, `hx-push-url="true"` en el form de `/timeline`; verificado con curl combinando los 4 filtros a la vez)

**Notas de implementación (decisiones no explicitadas por el plan):**
- Los entregables (E1–E4) no tienen fecha de inicio, solo `due` — no hay "duración" que dibujar como barra. El Gantt se implementó como marcadores (círculos) por fecha de vencimiento en vez de barras, con flechas curvas de dependencia entre marcadores; se documenta aquí porque el plan asume "barras" sin aclarar que el modelo de datos no tiene fecha de inicio.
- Los IDs de entregable (`DLV-001`, `DLV-002`...) se reinician por actividad, así que `depends_on` solo tiene sentido dentro de la misma actividad. El grafo de at-risk usa una clave global `ActivityID#DeliverableID` para no colisionar IDs de distintas actividades.
- Detección de ciclos: `resolveAtRisk` (`internal/server/handlers_timeline.go`) usa un set "en curso" (no una pila con profundidad limitada) — si detecta que ya está resolviendo esa clave, corta la rama devolviendo `false` en vez de seguir. Cada entregable igual puede ser at-risk por su propia fecha aunque el ciclo no aporte nada.
- Diagnóstico organizacional (`riesgo de coordinación`, `candidata a revisión`, huérfana) vive en `internal/server/handlers_org.go`, no en un paquete nuevo — sigue el patrón de `handlers_matrix.go` (helpers puros y testeables en el mismo paquete que los handlers) en vez de crear una capa `internal/org` no prevista en la arquitectura de `CLAUDE.md`.
- El treemap es slice-and-dice de un solo eje (un único corte horizontal, altura constante): con ≤8 secciones configurables no se justifica alternar orientación por nivel (no hay jerarquía real, solo secciones planas), y mantiene el área directamente proporcional al ancho — más fácil de verificar a mano que un squarified.
- El panel de detalle al hacer click en una tarjeta de swimlane o en una fila del timeline reutiliza el handler existente `/matrix/impact-effort/detail/{id}` (Fase 2) en vez de crear un panel de detalle nuevo — no hay requisito de auditabilidad distinto para estas vistas y evita duplicar la plantilla `detail_panel`.
- El treemap enlaza a `/org/swimlanes?section=X` con navegación normal (`<a href>`, sin HTMX) — el criterio pedía "filtra el swimlane", no una actualización parcial, y una navegación simple ya deja la URL compartible/recargable.
- Dashboard home (`GET /`) reemplaza el redirect anterior a `/activities`; se agregó el link "Inicio" al nav (antes no existía ninguna entrada para la raíz `/`).
- Se borró `handlers_placeholder.go` y `placeholder.html.tmpl`: cubrían las 4 vistas de Fase 2/3 antes de implementarse y ya no les queda ningún caller tras esta fase (Fase 5 tarea 5, `/help`, se implementará con su propio handler cuando llegue).

### FASE 4 — Export + empaquetado + script Python opcional

**Tareas:**
1. `go get github.com/xuri/excelize/v2`. Export (`POST /export/xlsx`) → `export/portfolio_YYYYMMDD.xlsx` con hojas: Actividades (todas las respuestas + scores + cuadrantes), Deliverables, Matriz I/E (scatter chart nativo de excelize con líneas de cuadrante), Eisenhower (ídem), Resumen por sección. Respuesta: descarga del archivo + copia en export/.
2. Export SVG de cada diagrama (`GET /export/svg/{view}`) — servir el mismo SVG con headers de descarga.
3. Backup (`POST /export/backup`) → copia timestamped del JSON en export/.
4. `scripts/import_backlog.py`: lee un .xlsx (openpyxl) con columnas mínimas (nombre, tipo, sección, owner, deadline) → emite JSON compatible con el Store → se importa vía `POST /import` o escribiendo el archivo. Debe funcionar sin el script (opcionalidad, §2.1 de la propuesta). Incluir `scripts/README.md` con formato esperado y `requirements.txt` (solo openpyxl).
5. Empaquetado: script `build.ps1` + `build.sh` → `GOOS/GOARCH` para windows/amd64, darwin/arm64, linux/amd64 con `-ldflags "-s -w"`. Verificar que el .exe corre en máquina sin Go (sin dependencias runtime).
6. README.md: qué es, cómo ejecutar (doble click), dónde viven los datos, cómo respaldar, cómo importar backlog, FAQ del modelo localhost ("no es un servidor de red").

**Criterios de aceptación:**
- [ ] xlsx abre sin reparación en Excel; charts presentes; datos coinciden con la app.
- [ ] Binario windows compilado desde el script arranca y funciona con solo el .exe + data.
- [ ] Script Python importa un xlsx de prueba de 5 filas y las actividades aparecen (con flag "necesita info" activo porque faltan respuestas B–F: importar con F1=1).

### FASE 5 — Piloto y calibración

**Tareas:**
1. Ampliar seed a 20 actividades realistas distribuidas en los 4 cuadrantes de cada matriz.
2. Sesión de revisión con el usuario: cargar su backlog real (15–30 actividades) vía cuestionario o script de importación.
3. Calibrar pesos y umbrales con el usuario; documentar los valores elegidos en README.
4. Pulido UX detectado en piloto (lista abierta; timebox 1 día).
5. Documentación final: guía de usuario de 1 página (dentro de la app, ruta `/help`) con las anclas de cada escala y el significado de cada cuadrante.

**Criterios de aceptación:**
- [ ] Usuario valida que la clasificación de su backlog "se siente correcta" o los pesos se ajustaron hasta lograrlo.
- [ ] `/help` accesible desde el nav.

---

## 4. Convenciones técnicas transversales

- **HTMX:** respuestas parciales devuelven fragmentos HTML (sin layout); detectar `HX-Request` header para decidir fragmento vs página completa. Nada de JSON en el flujo UI.
- **Templates:** un layout base + bloque `content`; fragmentos en archivos separados `_*.html.tmpl`. Funciones template registradas: `norm`, `quadrant`, `fmtDate`, `pct`.
- **CSS:** variables custom para la paleta de secciones; diseño simple tipo sistema (sin dark mode en v1).
- **Errores:** handlers devuelven 422 con fragmento de error inline para validación; 500 con página de error genérica y log detallado en consola.
- **Fechas:** siempre `time.Time` UTC internamente, formato `2006-01-02` en UI y JSON.
- **Accesibilidad mínima:** SVGs con `<title>`/`aria-label`; formularios con `<label>`; contraste AA en la paleta.
- **Sin dependencias más allá de:** stdlib, excelize (solo Fase 4). Si tientas añadir otra, no lo hagas.

## 5. Riesgos de implementación y mitigaciones

| Riesgo | Mitigación en el plan |
|---|---|
| SVG server-side se vuelve inmanejable | Helpers de geometría centralizados en `internal/svg`; matrices comparten renderer |
| Estado del cuestionario multipaso se pierde | Hidden fields (stateless); test E2E manual del flujo completo en criterios F1 |
| Ciclos de dependencias cuelgan el at-risk | Detección de ciclos con set de visitados; test obligatorio |
| Pesos mal configurados rompen scores | Validación suma=1.0 en servidor; scores nunca persistidos |
| excelize genera charts corruptos | Criterio explícito "abre sin reparación"; fallback: hojas sin chart + SVG export |

## 6. Fuera de alcance (v1 — no implementar aunque parezca fácil)

- Multiusuario, autenticación, sincronización.
- SQLite (el JSON basta; la propuesta lo da como alternativa, no requisito).
- Opción B (workbook Excel puro) — solo existe como target de export.
- Drag-and-drop para reclasificar cuadrantes (la propuesta lo menciona; los sliders de umbral + edición de respuestas cubren el caso; evaluar en piloto si se pide).
- PNG export (SVG basta; PNG vía Python queda como extensión futura).
- Dark mode, i18n.
