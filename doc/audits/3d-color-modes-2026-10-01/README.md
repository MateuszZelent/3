# Shared 3D field coloring

Arrows, voxel and volume now expose the same visible Color by panel below the
viewport: vector orientation, magnitude and X/Y/Z components. Components and
magnitude use the shared 2D palette, a physical-unit legend and automatic or
manual range. Surface projection and surface lighting remain volume-only.
Arrow direction is unchanged by its color selection. Voxel thresholding uses
vector magnitude regardless of coloring, so selecting a zero component does
not remove the cells. Scalar quantities retain the existing volume behavior.

Validation: svelte-check reports zero errors/warnings; 27 frontend unit tests
pass; production build passes. The attached Chromium smoke test exercised a
real CUDA field (7x5x3) through the embedded production frontend: all five
vector color options in all three render modes, pixel differences between X/Z
and orientation/magnitude, manual range changes, and mobile overflow. No
JS/WebGL console errors were observed. The fixture has m vectors varying in X,
positive Y and zero Z; therefore it also checks coloring a zero component.

The browser test validates this fixture, not the full 500-cube YIG simulation.
