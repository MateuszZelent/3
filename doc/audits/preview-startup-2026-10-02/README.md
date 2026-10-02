# Initial preview and reload initialization

Main-state mesh metadata and field frames arrive on separate websockets. The
Visualization component is also imported asynchronously. Previously a field
frame could start a renderer before mesh metadata or a mounted/sized surface.
The component had a second direct render path on mount, while receiving mesh
metadata did not schedule a render. A paused simulation could retain that first
incomplete view until another action produced a preview update.

The websocket render scheduler now owns initial rendering. Visualization
registers its mounted canvas and requests a redraw on surface resize. A renderer
requires a connected surface with positive dimensions and valid mesh counts/cell
sizes. Frames arriving early remain in the preview store and are acknowledged;
mounting or receipt of changed mesh geometry renders the stored frame without
waiting for new field data. Readiness is rechecked after lazy imports to handle
unmounts. Unchanged mesh telemetry does not trigger extra rendering.

Verification:
- 44 frontend unit tests pass; startup cases cover all six frame/mesh/mount
  permutations in 2D and 3D, zero-size surfaces becoming visible, changed versus
  unchanged mesh geometry, and unmounting during a lazy import.
- Svelte check: zero errors/warnings; production frontend build; webui/CLI tests
  and local executable build in the supported CUDA container.
- Real GPU backend + Chromium: Volume, Arrows, Voxel and 2D, each on first load
  and reload. The test delays the Visualization chunk, withholds mesh metadata,
  delivers exactly one initial field frame, and confirms initialization after
  releasing the metadata. It then receives a fresh forced-refresh frame and
  verifies that the two canvas screenshots are byte-identical. No view-mode
  interaction is used to initialize the UI. See browser-result.json.
- Control on the published v3.12.17-20261002 executable fails: its renderer starts
  before mesh metadata. With that guard bypassed, its initial Volume canvas
  differs from the subsequent refreshed image. The recorded failures are
  intentional regression evidence, not failures of the patched build.

Local artifact: build/mumax3-preview-startup. Included in release
v3.12.18-20261002. The browser script uses installed frontend Playwright dependencies
and requires a running interactive backend; PREVIEW_URL defaults to port 35383.
