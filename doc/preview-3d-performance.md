# Large-volume 3D preview

## Changes

- `Z data points` is available for vector previews in All layers mode. Its values
  divide the source depth, just as the X/Y choices divide the plane dimensions.
  The selected count is retained when switching back to Single layer.
- Automatic reduction distributes the point budget over all nontrivial axes.
  At the default 131072-point budget, both 250³ and 500³ become 50³ (125000
  points). Previously 250³ became 10×10×250 (25000 points).
- Scene coordinates and the camera use physical mesh dimensions, normalized by
  a single common scale. Independent sampling no longer stretches a cube into
  a column. Changing the sample count also no longer resets the camera.
- Z samples come from the middle layer of each depth bin. The renderer places
  them at bin centers and scales voxels to bin dimensions. X/Y continue to use
  the existing CUDA block average. Z is representative sampling, not a volume
  average; fine structures between sampled layers can be missed.
- Voxel subsampling operates on sampled Z indices rather than original layer
  indices. This avoids aliasing between the backend stride and the voxel
  sampling step.
- The backend reuses its vector-value allocation and traverses each selected
  layer in contiguous X order. Reduced Z means fewer CUDA launches: the vector
  path launches three layer kernels per selected layer, followed by one batched
  device-to-host copy. For 250³ at the default budget, launches fall from 750
  to 150 per preview update.
- WebGL uploads cover only live matrices/colors rather than the entire reserved
  power-of-two capacity. Replacing an InstancedMesh disposes its instance buffers
  as well as its geometry/material.

## Remaining costs and useful next steps

The renderer already uses instancing and renders on demand. The WebSocket client
coalesces preview updates per animation frame, and the server discards stale
queued frames. These mechanisms should be preserved.

Preparation still performs O(N) CPU work for orientations, matrices and colors
on every received frame. A custom instanced shader could accept positions and
vectors directly and compute orientation/color on the GPU. This would reduce
JavaScript work and matrix uploads, but requires validating the existing color
mapping, lighting, voxel filters and topography.

Every frame deliberately includes positions so it can be decoded even after
frames are dropped. At 125000 points, positions and vectors alone occupy about
3 MB before transport compression (24 bytes per point). Omitting unchanged
positions requires a reliable topology resynchronization protocol; simply
removing them would break clients that missed a topology update.

The transparent voxel material draws internal faces and can become fill-rate
limited. Surface extraction or opaque rendering would be separate visualization
modes with different semantics. The current voxel view preserves transparency.

Disabling automatic scaling permits the full requested volume, including 125
million points for 500³. Efficient interactive inspection relies on the preview
budget or explicit XYZ reduction. The simulation grid and saved data are not
changed by these controls.

## Verification

- `go test ./webui -count=1`: passed, including equal-axis budgets, thin-volume
  budget handling, manual Z selection and Single layer isolation.
- Frontend unit tests: passed, including physical proportions and anisotropic
  cell dimensions.
- `npm run check`: zero errors; existing ViewCube accessibility warning.
- `npm run build`: passed.
- Chromium integration test: passed with synthetic 5000-vector data; checks
  visibility and API payload of the Z control, actual instance coordinates,
  camera target and switching between arrows and voxels.

This is code and functional verification, not an end-to-end performance
benchmark on the live remote simulation. No FPS or wall-time speedup is claimed.
