# Visible 3D field controls

The shared Color by panel now precedes the canvas. Arrows and voxel open it by
default, including narrow screens. Arrow direction remains the complete XYZ
vector; coloring selects orientation, magnitude or a single component.

Voxel material/topography tools moved out of the nested advanced appearance
panel into an open panel beside coloring. Opacity, spacing, strength threshold,
sampling, topography axis and amplitude remain available. The controls area
scrolls independently on small viewports to retain room for the canvas.

Checks: Svelte check (zero errors/warnings), 27 frontend unit tests and production
build passed. The attached native CUDA/Chromium test verified the color selector
above the canvas in all three modes, five color modes and manual range, visible
voxel sliders, enabling/disabling topography and choosing its displacement axis,
and the mobile arrow color selector without horizontal overflow or JS/WebGL errors.
Evidence applies to the local build `build/mumax3-visible-panels` and a 7x5x3
fixture, not the deployed HPC process or an already published release.
