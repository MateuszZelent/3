# WebUI without connected clients — 2026-10-02

Scope: source changes after v3.12.19, prepared for v3.12.20. Runtime evidence
below comes from a local build. Verification of the published artifact is
recorded separately in its GitHub release notes.

## Findings and fixes

- `--webui-disable` / `-http=""` bypasses WebUI startup entirely.
- The periodic preview was already gated by active preview clients. It did not
  routinely evaluate quantities or download CUDA data without a client.
- WebUI initialization nevertheless launched CPU/OS counters and `nvidia-smi`
  once. Metrics now start only when a main WebSocket client requests telemetry.
- The broadcaster used to wake every second even with no connections. It now
  sleeps on a connection-change channel, with no polling timer while idle.
  Connecting a client wakes it; removing the final client suspends it again.
- Publication triggered by API changes now checks recipients before capturing,
  updating other sections or encoding. Preview clients that unsubscribe or
  await a render ACK cannot trigger new periodic captures.
- Preview capture and region-map enumeration recheck client demand on the
  CUDA-owning engine thread. A request queued behind solver work is skipped if
  the client disconnects before the task executes.
- The unused combined preview/main broadcaster was removed. Broadcaster startup
  follows state initialization, before the HTTP server accepts connections.

No UI capture, field evaluation, preview CUDA kernel or GPU-to-CPU preview copy
starts without a preview recipient. Region-map downloads likewise require a
main recipient. A capture already executing when disconnect occurs can finish;
an already running metrics subprocess has a bounded timeout. Reusable preview
GPU/host buffers are retained for reconnect, so this is not a claim of zero
retained VRAM. The listening HTTP server also retains a small idle footprint.

Solver output remains independent of UI clients: `TableAutoSave`, `AutoSave`,
explicit field evaluation, core tracking and enabled FFT must still execute
when requested by the simulation. Table history is also used by Zarr/HDF5
output and by later UI connections; recording the already computed scalar
values does not introduce another GPU read. Log history supports both output
initialization and later console connections. These histories are not disabled.

## Verification

Supported CUDA 12.4 / Go 1.26.8 container:

- Race tests cover zero recipients, unsubscribed clients, pending render ACKs,
  disconnect/reconnect/shutdown, lazy metrics, and queued preview/parameter
  requests cancelled before execution.
- Idle tests leave `engine.Inject` unserved and reject any solver injection;
  incomplete main state also makes accidental main-state processing fail.
- `go test -vet=off ./webui ./cmd/mumax3`: passed.
- Compiled local solver ran on RTX 4080 SUPER. A PATH wrapper recorded external
  `nvidia-smi` invocations: zero before the first browser connection; telemetry
  was invoked after connection. No CUDA profiler was used, so absence of preview
  GPU transfers is established by the guarded code path and injection tests,
  not by an independent device trace or a throughput benchmark.
- Chromium exercised first load/reload with delayed mesh metadata for 2D,
  Volume, Arrows and Voxel. All eight cases passed (startup-result.json).

The browser harness is shared with
`../preview-startup-2026-10-02/startup-browser.mjs`; set `PREVIEW_URL` to the
local test solver. Local runtime evidence is in `/tmp/3-ui-idle-proof`.
