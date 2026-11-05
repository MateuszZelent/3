import { describe, expect, it } from 'vitest';
import { previewFrameMesh } from './previewFrameMesh';

const mesh = (nx: number, dx: number) => ({
	dx,
	dy: dx,
	dz: dx,
	Nx: nx,
	Ny: nx,
	Nz: 1,
	Tx: 0,
	Ty: 0,
	Tz: 0,
	PBCx: 0,
	PBCy: 0,
	PBCz: 0
});

describe('atomic preview frame mesh', () => {
	it('keeps restored frame geometry when the global mesh is newer', () => {
		const displayed = mesh(100, 2e-9);
		const current = mesh(900, 7e-9);
		expect(previewFrameMesh({ mesh: displayed }, current)).toBe(displayed);
		expect(previewFrameMesh({}, current)).toBe(current);
	});
});
