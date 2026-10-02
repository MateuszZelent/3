# Preview window and demagnetization UI

The backend now initializes the 3D render-window state before the first preview
frame. Previously an empty region mode prevented its mesh bounds from being
initialized, while the controls depended on that state and started collapsed.
X/Y/Z ranges are open by default in the docked workspace and available in floating
and fullscreen viewports. Cropping precedes averaging for all three render modes.
Local windows receive the complete prepared grid without extra transport or
display sampling, including custom resolution. Native cells retain the existing
one-million-point safety limit; shrink the window to recover exact cells.

The main resolution control is labelled Preview point budget. The separate,
optional browser-transfer limit appears only in an advanced disclosure for
full-domain Arrows/Voxel; Volume and local windows always receive the complete
prepared grid. These settings do not change the simulation mesh.

Demagnetization initialization has an independent, mutex-protected progress
snapshot and polling endpoint. It does not acquire the engine or websocket-state
locks. The floating UI panel shows elapsed time, current-stage progress, cache
and GPU logs, and can be minimized. Completion auto-closes; errors remain visible.
Logs are bounded to 80 entries. Terminal progress hiding does not suppress UI
progress. No cancellation is offered for non-interruptible GPU initialization.

Validation on local RTX 4080 SUPER with the supported CUDA container:
- Svelte check: zero errors/warnings; 29 frontend unit tests; production build.
- Go util/mag race tests, webui and CLI tests. Tests cover progress concurrency,
  copied/bounded history, stale operation IDs, hidden terminal bars and an HTTP
  progress read while the websocket-state mutex is held.
- Chromium with the 4096x8x4 long-window fixture: full grid 100x8x4; native
  32x8x4 middle window, 992 occupied cells (cavity excluded), analytic per-cell
  values in all render modes. Panning, fractional custom averages, XYZ bounds,
  scalar alpha, 2D/3D round-trip, mobile fullscreen, full-domain restoration. The single primary budget and advanced-only transfer controls are checked through the UI.
- Chromium with 128 cubed mesh (padded 256 cubed, 384 MiB cache): seven live
  updates during blocked engine work, including calculation, writing, GPU plans
  and transformation; successful completion and no browser errors.

Evidence here is from the locally built executable build/mumax3-ui-window.
No 500-cube or remote HPC run was performed. Current-stage percentages are not
an estimated total-startup percentage. Browser scripts use the frontend's local
Playwright installation and require a running interactive simulation.

Style audit: viewport chrome, 2D/3D controls, window sliders, orientation cube
and shared Panel/controls now resolve application surface/text/border/accent
tokens instead of a separate dark palette. The JavaScript canvas theme resolves
the same CSS tokens; Tailwind already maps its utilities to these tokens.
Chromium computed styles match surface-1/surface-2 for the default palette and
a synthetic light token override (this is not a new theme selector). Captured
3D canvas corner pixels match both palettes exactly: RGB 15/23/40 and 232/238/246.
Mobile layout has no horizontal overflow. Scientific color palettes and axis
colors remain independent of UI surface styling.
