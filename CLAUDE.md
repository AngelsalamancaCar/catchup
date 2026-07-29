# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Project Mapper: a local-only, single-binary Go + HTMX tool that captures activities via a questionnaire and produces an Impact/Effort matrix, an Eisenhower matrix, an org swimlane/treemap, and a deliverables timeline — all as server-rendered inline SVG. No JS framework, no database server, no network exposure (binds to `127.0.0.1` only).

The build is being driven by two documents at the repo root — **read them before making product/architecture decisions**:
- `proposal-project-mapper.md` — the requirements source of truth (questionnaire fields, formulas, data model). Ambiguity resolves in favor of this file.
- `plan-implementacion.md` — the phase-by-phase implementation playbook (§0 has agent operating rules, §2 has the normative scoring formulas, §3 has per-phase tasks and acceptance criteria). Work phase by phase in the order defined there; don't start a phase until the previous one's acceptance criteria pass.

## Commands

```bash
go build ./...                          # build everything
go vet ./...                            # vet everything
go test ./...                           # run all tests
go test ./internal/scoring/... -cover   # scoring package must stay >= 90% coverage
go test ./... -run TestName             # run a single test by name
gofmt -l .                              # list files needing formatting (fix with gofmt -w)
go run ./cmd/projectmapper -port 8787 -data data/projects.json -seed   # run locally
./build.sh   # or build.ps1 — cross-compiles dist/projectmapper-{windows-amd64.exe,darwin-arm64,linux-amd64}
./scripts/install-shortcut.ps1 [-NoBuild|-Remove]   # Windows desktop/Start-Menu shortcut → %LOCALAPPDATA%\Catchup
go run ./scripts/mkicon   # regenerates assets/catchup.ico (offline generator, only if the palette changes)
```

There's no Makefile — the commands above are the whole toolchain. After any change, `go build ./...`, `go vet ./...`, and `go test ./...` must all pass cleanly before considering work done.

`-seed` copies `data/seed.json` into `-data`'s path only if that file doesn't already exist (never overwrites real data). For manual UI verification, run the binary on a scratch `-data` path/port, `curl` the routes that changed, then kill the process — don't leave a stray listener from a previous session.

## Stack ceiling (do not add dependencies)

Go stdlib + `github.com/xuri/excelize/v2` only (excelize is export-only, used from `internal/server/handlers_export.go`, never from `scoring`/`svg`). Vendored HTMX (`web/static/htmx.min.js`, fetched once). Python is allowed only for the optional Phase 4 backlog-import script (`scripts/import_backlog.py`). **No Node, no JS framework, no router library, no ORM, no CGO, no SQLite** — the plan explicitly forbids reopening these decisions.

## Architecture

### Layering

```
cmd/projectmapper/main.go   → flags, opens browser, starts HTTP server on 127.0.0.1 only
internal/model               → plain structs (Store, Activity, Answers, Weights...), zero logic
internal/store                → JSON persistence: atomic write (temp file + rename), RWMutex, sequential ACT-NNN IDs
internal/scoring               → pure functions: Answers + Weights + now → scores. No I/O, ever.
internal/svg                    → pure geometry: score (0-100) → SVG pixel coords, color palettes, radii
internal/server                  → net/http mux, handlers, html/template rendering
web/                               → embed.FS'd templates (*.html.tmpl, *.svg.tmpl) and static assets
```

Dependency direction is strict: `server` depends on `scoring` and `svg`; `scoring` and `svg` never import `store` or `server`. Scores are **never persisted** — `store.Store` only holds `Answers` + `Weights` + `Thresholds`; every score is recomputed on read from those three, via `scoring.Compute(activity, weights, thresholds, now)`. This is intentional (plan §2.1): it keeps weight/threshold changes from ever going stale.

### The scoring engine (`internal/scoring/scoring.go`)

Every score function (`ImpactScore`, `EffortScore`, `UrgencyScore`, `ImportanceScore`, `UncertaintyScore`) returns a `Result{Score float64, Breakdown []Component}` — the breakdown is the auditability requirement from proposal §7 ("every computed score traceable to questionnaire answers"): each `Component` carries raw answer, weight, normalized value, and weight×normalized contribution, and the UI renders this table in the detail panel of both matrices. When adding or changing a score formula, keep this breakdown structure — don't collapse it into a bare float.

Normalization tables and formulas are specified exactly in plan §2.2–§2.4 (band mappings for C1/C4/D2/D3, the deadline-proximity ramp, quadrant classification, the ±8 "borderline" band, the info-need overlay rule). Match them exactly rather than approximating — the test suite in `scoring_test.go` checks the numbers, not just the shape.

`Compute()` is the one entry point handlers should call — it bundles both quadrant classifications, both borderline flags, and the `NeedsInfo` / `DecideWithWhatYouHave` overlay flags in one pass.

### SVG rendering (`internal/svg` + `web/templates/matrix_svg.svg.tmpl`)

`svg.BuildMatrix(MatrixConfig, []PointInput) MatrixView` is the shared renderer for both matrices (plan explicitly calls for extracting this common helper). It does **all** arithmetic in Go — pixel coordinates, threshold line positions, quadrant label anchors, badge offsets — so `matrix_svg.svg.tmpl` only interpolates already-computed fields and never does math. Keep it that way: if a matrix needs new visual behavior, compute it in `internal/svg` or the handler, not in the template.

### Templates (`internal/server/server.go` `loadTemplates`)

Each "page group" (e.g. `activity_wizard`, `matrix_impact_effort`) is parsed as its **own** `*template.Template` combining `layout.html.tmpl` with that page's file(s) — this is required because Go's `html/template` shares the `{{define "content"}}` namespace within one parsed set, so different pages must not share a parse group or their `content`/fragment names collide. Files reused across groups (`matrix_svg.svg.tmpl`, `activity_detail.html.tmpl`) get listed in multiple `mustParse(...)` calls, which is expected, not duplication to clean up.

`renderFull` executes `"layout"` (full HTML page); `renderFragment` executes a named sub-template directly (an HTMX partial response, no layout). Handlers decide which one to call based on why they were invoked (initial GET vs. an `hx-get`/`hx-post` step), not by sniffing `HX-Request` inside shared render code.

### The questionnaire wizard (`internal/server/wizard.go` + `activity_wizard.html.tmpl`)

Deliberately **stateless**: there is no server-side session for an in-progress questionnaire. Each step's form re-submits every prior step's answers as hidden fields (`hidden_A`, `hidden_B`, ... named template blocks), and each handler (`wizardAdvance`) re-parses the *entire* accumulated form, validates only the step just submitted, and re-renders either the next step (success) or the same step with inline errors (failure, values preserved). Editing reuses the identical wizard with `Mode: "edit"` and the activity's answers pre-loaded into the same field names. If you add a new question, it has to flow through: the `WizardForm` struct, a `parseWizardForm` field, a `validateStepX` check, the relevant `hidden_X` block (so later steps carry it forward), and `toActivity()`/`wizardFormFromActivity()` (so it round-trips to/from `model.Answers`).

### Deliverables repeating group

Added via `hx-get /activities/wizard/deliverable-row?index=N` which returns the new row **plus** an out-of-band (`hx-swap-oob`) replacement of the "+ Deliverable" button with the next index baked into its URL — this is how the client-side index counter stays in sync without any server session. Deliverable rows are parsed server-side by scanning `deliverables[i].name` for `i` up to `maxDeliverables`, not by trusting a submitted count field.

### Persistence (`internal/store/store.go`)

Single JSON file, guarded by one `sync.RWMutex`. Every mutating method (`CreateActivity`, `UpdateActivity`, `SetWeights`, `SetThresholds`, ...) writes synchronously via `atomicWrite` (temp file in the same dir + `os.Rename`) while holding the lock — there's no separate `Save()` call to remember. `Snapshot()`/`GetActivity()` return deep copies (JSON round-trip) so callers can't mutate internal state through an alias. IDs are `ACT-NNN`, assigned by scanning existing IDs for the current max and incrementing — not reused after deletes.

### Type-based routing (proposal §4.3)

Impact/Effort matrix defaults to showing only `Project`/`Workstream` activities; Eisenhower defaults to `Recurring`/`AdHoc`. Both have a `show_all=1` query param toggle. This filter is applied in the handler (`isImpactEffortType`/`isEisenhowerType` in `handlers_matrix.go`), not in `scoring` or `svg` — those packages stay activity-type-agnostic.

### `/help` (plan §3 Fase 5, tarea 5)

Static one-page user guide (`internal/server/handlers_help.go` + `web/templates/help.html.tmpl`), no query params or server-side state. It reuses the `anchors()` template func already used by the questionnaire wizard for the 1–5 scale descriptions, instead of re-typing them — if a scale anchor changes, `template_funcs.go` is the only place to edit and both the wizard and `/help` pick it up. The rest of the page (formulas, quadrant meanings, org diagnostics, timeline at-risk rule) is copied 1:1 from plan §2.2–§2.4 and §4, not from the proposal, since the plan is the exact-numbers source.

### Desktop launch path (`cmd/projectmapper/main.go` + `scripts/install-shortcut.ps1`)

`scripts/install-shortcut.ps1` copies `dist/projectmapper-windows-amd64.exe` to `%LOCALAPPDATA%\Catchup\catchup.exe` and creates a `Catchup.lnk` (desktop + Start Menu) whose `Arguments` carry an **absolute** `-data` path and whose `WorkingDirectory` is the install dir — that's what keeps `exportDir` (relative, `handlers_export.go`) writing under the install dir instead of wherever the shortcut was launched from. The `.lnk` runs minimized (`WindowStyle = 7`) because the console window *is* the stop button; there is deliberately no `/quit` route and no `-H windowsgui` build.

Because a shortcut-launched console disappears the instant the process exits, `main.go` never uses `log.Fatalf`: startup failures go through `fatal()`, which pauses on stdin (EOF-safe, so pipes and CI don't hang). And since the second double-click is the most likely failure, `main` binds via `net.Listen` before serving: if the port is taken, `alreadyRunning()` probes the home page for the layout's "Project Mapper" marker and, on a hit, just opens the browser at the live instance and exits 0.

`assets/catchup.ico` is produced by `scripts/mkicon` — an **offline** stdlib-only generator (ICO container with embedded PNGs at 16/32/48/256) that reads its colors from `svg.SectionPalette`. It's committed, so neither the binary nor the installer depends on it; re-run `go run ./scripts/mkicon` only if the palette changes.

### Seed data (`data/seed.json`)

20 activities, 5 per `Type`, deliberately composed so the **default-filtered** view of both matrices (not just `show_all=1`) already shows all 4 quadrants: Impact/Effort needs Project/Workstream activities in Quick Wins, Major Projects, Fill-Ins, *and* Thankless Tasks; Eisenhower needs Recurring/AdHoc activities in Do, Schedule, Delegate, *and* Delete. Before the Fase 5 seed expansion, the default views had zero Quick Wins/Thankless Tasks and zero Schedule — worth checking again if activities are ever removed from the seed. It also exercises diagnostics that had no seed coverage: one activity (`ACT-020`) uses a `section` not present in the top-level `sections` list, to populate the orphaned-activity swimlane; two activities land in Legal as Thankless Tasks, to trigger the "candidata a revisión" section badge. If you add/remove seed activities, re-verify quadrant coverage by running the binary against a scratch `-data` path and inspecting `GET /matrix/impact-effort/svg` and `GET /matrix/eisenhower/svg` (each point's `<title>` includes its computed scores) rather than eyeballing the JSON.

## Conventions

- UI language is Spanish; Go identifiers, comments in code, and commit messages are English/Spanish-mixed following what's already there — match the file you're editing.
- Dates: `time.Time` UTC internally, `"2006-01-02"` on the wire (form fields, JSON, template output via the `fmtDate` template func).
- Commit messages: `feat(fase-N): <summary>` per completed plan phase (see git log for the established tone/format).
- Section colors: the 5-color mono-accent palette in `internal/svg/geometry.go` (`SectionPalette`) and the CSS custom properties `--section-1..5` in `web/static/app.css` must stay in sync if either changes.
