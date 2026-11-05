# MuMax3 WebUI development

Use the Node and npm versions pinned in `package.json` and CI. Install the locked
dependencies with `npm ci --ignore-scripts`.

```sh
npm run lint
npm run check
npm run test:unit
npm run build
npx playwright install chromium --only-shell
npm run test:production
npm run test:e2e -- --project=chromium --workers=1
```

`build` also updates `../webui/static`, which Go embeds. Rebuild the executable
after rebuilding the frontend. Do not run `build` against the same working tree
while a Vite development test server is running: both use `.svelte-kit`.

Browser tests use isolated HTTP and WebSocket fixtures. An unexpected API call or
an unhandled page exception fails the test. Tests must never send solver commands
to an existing development backend. Screenshot baselines use Linux Chromium and
mock external fonts; review intentional visual changes before updating them.

## HTTP and WebSocket contracts

- Preview mutations return `{requestId, configRevision}`. Region updates include
  the applied region and the same acknowledgement fields. A preview frame carries
  `configRevision`, `sequence`, and mesh geometry from its own engine capture.
  A transition completes only after the matching frame has actually rendered.
- Changing a scalar 2D view to vector 3D can atomically send
  `{component: "3D", allLayers: true, requestId}` to `preview/component`.
- `preview` frames distinguish the configured grid from transport sampling:
  `transportSampling`, `serverDataPointsCount`, and `dataPointsCount` describe the
  transmitted subset. Sparse 2D tuples retain their original cell indices.
- Solver `run`, `steps`, `relax`, `minimize`, and console `command` return HTTP 202
  with `{operationId, status: "pending"}` when accepted. Acceptance does not mean
  completion. Main-state WebSocket messages publish each lane's `operationId`,
  `operationStatus`, and `operationError` under `solver` or `console`.
- Operation status is `idle`, `pending`, `running`, `completed`, `failed`, or
  `cancelled`. A lane accepts one outstanding operation; another submission gets
  HTTP 409. The console lane remains available while the solver lane is active.
  `solver/break` cancels pending work in both lanes and interrupts running work.
- A network failure or client timeout does not prove that a mutation was rolled
  back. Never automatically retry solver commands. Reconcile with server state;
  preview transitions provide an explicit retry/resync path.

## Renderer benchmark

Run `node scripts/preview-renderer-performance.mjs` from `frontend`; the script
starts and stops its own Vite server. Set `PREVIEW_BENCH_PORT` to choose its port
and `PREVIEW_BENCH_OUTPUT` to choose the JSON output path. The harness uses
synthetic frames and does not connect to a solver. It checks source coordinates,
values, camera/color updates,
and rendering quality while recording timings at 128k and 1M points. Its timing
mode disables WebGL drawing to isolate application preparation costs; it is not
a GPU throughput benchmark. Report wall time and main-thread long tasks
separately, and compare runs with identical sampling and visible instance counts.
