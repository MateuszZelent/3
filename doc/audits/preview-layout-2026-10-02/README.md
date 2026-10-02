# Visualization layout and control consolidation

The ViewCube and axis gizmo were positioned against the whole preview panel.
Its heading, field controls, optional voxel tools and statistics changed the
canvas position without changing those offsets. The cube could therefore appear
among controls, and axes could escape the rendered scene. Fixed-height floating
modes also gave the canvas the whole panel height rather than its remaining space.

The preview now has four explicit areas: heading, scrollable controls, scene
viewport, and statistics. Canvas, ViewCube, axes, empty state and transition
notice share the viewport, isolated from the controls. Floating/fullscreen modes
allocate the remaining space to this viewport. Gizmos have room for their
projected faces/labels and are also available for scalar 3D quantities.

Field coloring/projection, mode-specific appearance, and the 3D X/Y/Z window each
have one collapsible section above the scene, shared by inline, floating and
fullscreen views. The legacy floating toolbar and its separate display-only clip
state are removed; the backend 3D window controls spatial bounds and local detail
in all render modes. Lighting appears once in Surface appearance. Voxel material,
sampling and topography remain available in Voxel material & topography. Advanced
settings below the scene contain resolution, quality and optional transfer limits.
Surface/Volume defaults to Vector orientation; scalar quantities use field values.

The workspace uses two independent vertical columns. The primary column contains
Visualization followed immediately by Solver. Plots, Console, Mesh and Parameters
flow independently in the second column. On small screens the columns stack, so
Solver still follows Visualization. Diagnostics span the full workspace width.

Verification on the local Go 1.26.8/CUDA binary and actual RTX 4080 SUPER backend:
- 44 frontend unit tests; Svelte check zero errors/warnings; production build.
- Chromium: containment of cube faces, axes and labels within the canvas, with
  controls/statistics outside it, for Volume/Arrows/Voxel in inline, popout and
  fullscreen at desktop and mobile widths (18 combinations).
- Two additional sets of interactive checks: one appearance/window section each,
  lighting enabling its intensity control, a real local-window POST after editing
  a cell bound, restoring full domain, voxel topography/component controls, and
  scalar quantities retaining the camera gizmo (22 report entries total).
- The plot column was artificially extended to 4000px; Solver remained exactly
  one configured panel gap below Visualization on both widths.
- First-load/reload regression in all four 2D/3D modes (eight cases), including
  deliberately delayed mesh metadata and matching initial/refreshed canvas images.

Scripts and JSON results are stored alongside this report. PREVIEW_URL selects
the interactive backend (default port 35387 for the layout script). Included in
v3.12.19-20261002; these results describe local source verification, before the
published binary's separate smoke checks.
