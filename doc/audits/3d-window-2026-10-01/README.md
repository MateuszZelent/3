# 3D render window and preview transitions — 2026-10-01

The render window crops source cells on CUDA before resampling or transferring
preview buffers. Its X/Y/Z bounds are inclusive/exclusive source-cell edges.
Native cells use a separate 1,000,000-point safety limit; custom resolution has its
own point budget. Full-domain resolution and the previous single/all-layer mode
are retained. Geometry occupancy is cropped with the same kernel as field data.
Quantity evaluation still uses the solver's full field; this feature limits the
preview reduction, transfer and rendering, not the simulation domain.

The renderer uses local coordinates with physical proportions, so a small window
far from the mesh origin keeps float precision. Surface values and thickness
averages apply to the selected window. Dragging the selection pans without
changing its width. Requests are debounced and serialized; an older response
cannot replace a newer slider draft. Numeric cell bounds allow exact selection.

Changing quantity in Volume switches geometry-only shading to field shading and
resets the color range for the new quantity. Nearly constant Float32 fields
use a stable color range to avoid amplifying averaging roundoff into false bands. Scalar quantities such as alpha use
the value shader. Explicit geometry shading remains selectable afterward.
View and quantity requests show backend preparation and rendering phases. Large
2D heatmaps render in 4,096-cell chunks, and the websocket ACK waits for ECharts'
finished event. Rendering is serialized to prevent incoming frames restarting
an incomplete large heatmap. Small heatmaps retain atomic updates.

## Verified

- New CUDA kernel compiled for CC 50, 52, 53, 60, 61, 62, 70, 72, 75, 80, 86,
  87, 89 and 90; the RTX 4080 SUPER executed native and fractional XYZ crops.
- Independent CPU cell-intersection reference matches all 3 vector components
  for a nonzero origin and non-divisible output bins, including native cells.
- Go backend and command tests passed; frontend check had zero diagnostics,
  27 unit tests passed, and the embedded frontend and Go application built.
- Existing browser regressions for planes, long 2D windows, solid cells,
  cavities, occupied zeros, partial Z bins and physical proportions passed.
- The interaction browser test used a 450 ms backend delay and a 262,144-cell
  plane: both progress stages appeared; completion took 6,985–10,334 ms and
  the largest measured UI heartbeat gaps were 645–658 ms in local headless
  Chromium runs.
  This is a fixture measurement, not a guarantee for arbitrary simulation sizes.
- Native browser evidence in `native-browser.json`: mesh 4096 × 8 × 4;
  source X window [2040,2072) becomes 32 × 8 × 4 native cells instead of the
  full-domain 100 × 8 × 4 reduction. The 32-cell cavity is retained, leaving
  992 occupied cells in Volume, Arrows and Voxel. Keyboard pan shifts the source window by 10 cells;
  every returned native magnetization matches the analytic source field.
- Fractional local 7 × 8 × 3 averages match an independent CPU reference.
  All X/Y/Z controls are exercised. Alpha is verified from CUDA values through
  the selected surface shader. Six screenshot samples across the constant-alpha
  surface were all RGB (32, 135, 154), confirming that Float32 averaging noise
  no longer creates false color bands. 2D/3D round trips retain the window and Full
  domain restores 100 × 8 × 4. Mobile fullscreen is checked for canvas space,
  horizontal overflow and JavaScript errors.

## Reproduce

Run `native-browser.mjs` against the interactive built application using
`long-window.mx3`; it defaults to http://127.0.0.1:35367. Use `-http
0.0.0.0:35367` inside Docker and publish the port on 127.0.0.1. Screenshots are
written to `/tmp/3-region-*.png`.

The saved `preview-interactions.spec.ts` can be copied into `frontend/tests/`
(the local test directory is ignored) and run with the repository Playwright
configuration. Backend tests are in `webui/preview_region_test.go`; real GPU
reference tests are in `cuda/resizepreviewregion_test.go`.
