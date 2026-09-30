// World coordinates use one common physical scale on all axes. Changing
// preview sample counts must never change the shape of the simulated body.
export function preview3DLayout(
	mesh: { Nx: number; Ny: number; Nz: number; dx: number; dy: number; dz: number },
	samplesX: number,
	samplesY: number,
	layerStride: number,
	allLayers: boolean
) {
	const nx = Math.max(mesh.Nx, 1);
	const ny = Math.max(mesh.Ny, 1);
	const nz = Math.max(mesh.Nz, 1);
	const dx = mesh.dx > 0 ? mesh.dx : 1;
	const dy = mesh.dy > 0 ? mesh.dy : 1;
	const dz = mesh.dz > 0 ? mesh.dz : 1;
	const unit = Math.max(nx * dx, ny * dy, (allLayers ? nz : 1) * dz) / 100;
	const xSize = (nx * dx) / unit;
	const ySize = (ny * dy) / unit;
	const zCell = dz / unit;
	const depthCells = (allLayers ? nz : 1) * zCell;
	const stepX = xSize / Math.max(samplesX, 1);
	const stepY = ySize / Math.max(samplesY, 1);
	const stepZ = zCell * Math.max(layerStride, 1);
	return {
		xSize,
		ySize,
		depthCells,
		stepX,
		stepY,
		stepZ,
		zCell,
		glyphScale: Math.min(stepX, stepY, ...(allLayers ? [stepZ] : []))
	};
}
