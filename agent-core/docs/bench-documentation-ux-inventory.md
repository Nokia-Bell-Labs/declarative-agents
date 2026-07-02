# Bench Documentation UX Inventory

This inventory records the current bench documentation browser before it moves
to a standalone Knowledge Manager host. It covers `agent-core-h0x9.1.1`. No code
moved for this issue.

## Backend Ownership

`internal/evaluation/bench/server.go` mounts the HTTP routes and wraps API
responses with a `data` field on success or an `error` field on failure. The
docs repository logic lives in `internal/evaluation/bench/api_docs.go`.
Configuration overlays come from `internal/evaluation/bench/api_configs.go`.
Source overlays use `internal/evaluation/bench/api_source.go`.

Factory setup is split across two files. `internal/evaluation/bench/factories.go`
creates `BenchState` from the selected `serve_ui` declaration and normalizes
configured directories to absolute paths. `internal/evaluation/bench/serve_ui.go`
can fill unset `BenchState` fields from the same declaration when the server
state already exists. `state.go` maps frontend actions to
`ExperimentRequested`, `Shutdown`, or `CommandError`.

Asset loading stays under `internal/evaluation/bench/ui`. In normal builds,
`assets_dev.go` serves `internal/evaluation/bench/ui/dist` through `os.DirFS`.
With the `production` tag, `assets_prod.go` embeds `dist/*` and serves the
embedded subdirectory.

## Frontend Ownership

`internal/evaluation/bench/ui/src/App.tsx` owns the React routes and top-level
navigation. `api/client.ts` owns `/api/v1` helpers and the TypeScript response
shapes. `pages/Documentation.tsx` owns the docs browser, cross-linking, Mermaid
diagrams, YAML highlighting, raw YAML display, config overlays, and source
overlays.

`pages/TraceViewer.tsx` is not part of the docs browser, but it shows machine,
tool, and LLM config details from experiment artifacts. `pages/Launcher.tsx`
uses the config API to suggest suite files and submits launch actions. These
pages share the same embedded UI bundle, so standalone docs hosting must split
or intentionally retain that bundle boundary.

## Configuration

`agents/bench/profile.yaml` selects `agents/bench/builtin.yaml`. The `serve_ui`
tool config sets `addr`, `data_dir`, `configs_dir`, `docs_dir`, and
`profiles_dir`. It does not set `source_dir`, even though the backend and
frontend support source overlays. Without a configured `SourceDir`, source
overlay requests fail and the UI offers a GitLab fallback link.

`agents/bench/machine.yaml` treats `serve_ui` as the blocking human-boundary
word. Startup moves from `Idle` to `Serving`, `serve_ui` waits for a user action,
and `launch_eval` runs when the action emits `ExperimentRequested`.

## API Contract

Every API route sits under `/api/v1`, while non-API routes use the SPA handler
and fall back to `index.html` when the requested asset does not exist. `GET /api/v1/docs`
returns the docs index. Each entry has `path`, `name`, and
`category`; the path is relative to `docs_dir` and slash-normalized, the name is
the base filename without its extension, and the category is derived from the
relative path. `GET /api/v1/docs/{path...}` returns one document with `path`,
`content`, and `raw`. Parsed YAML fills `content` when parsing succeeds. YAML
parse errors do not fail the request; the handler returns raw text with a null
content value. Docs rendering also depends on `GET /api/v1/configs`,
`GET /api/v1/configs/{path...}`, and `GET /api/v1/source/{path...}`. Config detail
responses include parsed YAML, raw YAML, and an optional machine graph when the
YAML has `states` and `transitions`. Source responses include content, language,
MIME type, and byte size.

## Docs Repository Rules

An empty `docsDir` produces an empty docs list. Missing docs directories also
produce an empty docs list when the filesystem walk reports `os.IsNotExist`.
List output includes only `.yaml` and `.yml` files, then sorts by category
and path.

Category mapping is path based. Files directly under `docs_dir` are `overview`.
`specs/software-requirements/` maps to `srd`. `specs/semantic-models/` maps to
`semantic-model`. `specs/config-formats/`, `specs/use-cases/`, and
`specs/test-suites/` map to `config-format`, `use-case`, and `test-suite`.
Other nested files use the first path segment as the category.

Document fetch rejects empty paths, cleaned paths containing `..`, and paths
outside `docsDir` after join and prefix validation. Missing files return
`document not found`. Other read failures return `failed to read document`.

## Frontend Routes

React exposes docs routes at `/docs` and `/docs/*`. The selected doc path lives in
the React Router splat parameter. Selecting a sidebar entry routes to
`/docs/<doc path>` and then calls the get-doc API for that same path.

The same app also owns `/`, `/launch`, `/sessions/:suite/:ts`, and
`/sessions/:suite/:ts/points/:pointId`. Those routes do not belong to the docs
browser, but they remain coupled to the current bundle and build path.

## Rendering Behavior

Sidebar code groups entries by backend category and sorts groups by
`CATEGORY_META`. It opens the active category, shows the total document count,
formats SRD names with an uppercase prefix, and derives display titles from
filenames.

Document detail shows `title` as the heading and `id` as a monospace badge
when those fields exist. It gives priority to prose fields such as `purpose`,
`problem`, `overview`, `summary`, `executive_summary`, `what_this_does`,
`why_we_build_this`, `trigger`, `lifecycle`, and `pipeline_diagram`. Remaining
fields pass through recursive `YamlSection` logic.

Arrays with uniform object shapes become tables. Single-field object arrays
become field/value tables. Other arrays become list items or cards. Nested
objects recurse through the same renderer. Every document can expose raw YAML
inside a details block with highlight.js YAML highlighting.

Special handling exists for SRD requirement maps, semantic-model configuration
blocks, semantic-model state diagrams, `states` arrays, and `signals` arrays.
Mermaid fenced blocks inside text fields render inline. Semantic-model diagrams
can toggle between a generated model view and the configured machine definition
when `configuration.machine` points at a config file.

## Linking And Lookup

During load, the docs page builds an in-memory index from `DocEntry` values. It maps document
names, document paths, `docs/<path>` strings, SRD prefixes such as `srd020`, and
semantic model IDs matching `sm-...` to doc paths. Text linkification recognizes
doc-index entries, config paths under `configs/`, and source paths under `pkg/`
or `cmd/`.

Config file links open an in-page YAML overlay through the config API after
removing the `configs/` prefix. Config directory links open the configured
GitLab tree URL. Source file links open an in-page source overlay through the
source API. Source overlay failures show `Failed to load source file (is
--source configured?)` and a GitLab fallback link.

Current source matching recognizes only `pkg/...` and `cmd/...`. It does not
link `internal/...`, `agents/...`, or `tools/...` source paths.

## Asset And Build Impact

Development mode still expects a built `dist/` directory. The current path does not proxy a
Vite dev server. Production mode embeds the same `dist/` tree and uses
`index.html` as the SPA fallback.

`mage build` discovers `internal/evaluation/bench/ui/package.json`, runs
`npm install`, runs `npm run build`, and then compiles `cmd/agent` with
`-tags production`. A standalone documentation host needs its own asset
directory or a planned update to `embeddedUIDirs`.

## Test Impact

Executable coverage proves the bench machine loads, the bench tools
select `serve_ui` and `launch_eval`, the UI action transitions validate, session
APIs return evaluation artifacts, and the action endpoint applies single-action
backpressure. The formal `test-rel01.0` suite records `mage integration:uc003`
as skipped because browser-level bench UI proof still needs manual
preconditions.

Migration work has specific gaps. No focused Go test covers
`GET /api/v1/docs`, docs path traversal rejection, or source path traversal
rejection. There is no automated browser test for the docs route, sidebar,
Mermaid display, config overlay, or source overlay.

## Migration Contract

Extraction should preserve the success and error wrapper shape unless the UI
client changes with it. Keep `DocEntry.path`, `DocEntry.name`,
`DocEntry.category`, `DocDetail.path`, `DocDetail.content`, and `DocDetail.raw`.
Raw display still needs YAML parse tolerance. Category names, path traversal
rejection, and missing-file behavior should stay fixed unless the standalone
host updates the UI at the same time.

Follow-up issues need explicit choices on config overlays, source overlays,
source link roots beyond `pkg/` and `cmd/`, host scope for selected config and
source roots, and development asset serving through built `dist/` or Vite. That
decision should be made with the API extraction and asset host work, because a
copy-only migration would otherwise carry bench-specific config and source
assumptions into Knowledge Manager.

