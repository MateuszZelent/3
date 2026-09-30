import { describe, expect, it } from 'vitest';
import { preview3DLayout } from './preview3DLayout';

const cube = { Nx: 500, Ny: 500, Nz: 500, dx: 5e-9, dy: 5e-9, dz: 5e-9 };

describe('physical 3D preview coordinates', () => {
	it('keeps a cube cubic under independent XYZ sampling', () => {
		const layout = preview3DLayout(cube, 10, 50, 25, true);
		expect(layout.xSize).toBeCloseTo(layout.ySize);
		expect(layout.depthCells).toBeCloseTo(layout.xSize);
		expect(layout.stepX * 10).toBeCloseTo(layout.xSize);
		expect(layout.stepY * 50).toBeCloseTo(layout.ySize);
		expect(layout.stepZ * 20).toBeCloseTo(layout.depthCells);
	});
	it('preserves physical aspect ratio for anisotropic cells', () => {
		const layout = preview3DLayout({ ...cube, dz: 10e-9 }, 50, 50, 10, true);
		expect(layout.depthCells / layout.xSize).toBeCloseTo(2);
	});
	it('fits a single layer without including the hidden volume', () => {
		const layout = preview3DLayout(cube, 100, 100, 1, false);
		expect(layout.depthCells).toBeCloseTo(layout.zCell);
		expect(layout.glyphScale).toBeCloseTo(layout.stepX);
	});
});
