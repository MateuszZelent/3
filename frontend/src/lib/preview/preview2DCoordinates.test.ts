import { describe, expect, it } from 'vitest';
import {
	previewSampleCoordinateNm,
	previewPlaneAxes,
	fitPreviewPlane
} from './preview2DCoordinates';

describe('previewSampleCoordinateNm', () => {
	it('places preview samples at the centres of downsampled mesh bins', () => {
		expect(previewSampleCoordinateNm(0, 4, 400)).toBe(50);
		expect(previewSampleCoordinateNm(1, 4, 400)).toBe(150);
		expect(previewSampleCoordinateNm(3, 4, 400)).toBe(350);
	});

	it('keeps a one-pixel preview at the mesh centre', () => {
		expect(previewSampleCoordinateNm(0, 1, 400)).toBe(200);
	});
});

describe('physical preview planes', () => {
	it('maps both plotted axes and the perpendicular slice axis', () => {
		expect(previewPlaneAxes('xy')).toEqual({ u: 'x', v: 'y', normal: 'z' });
		expect(previewPlaneAxes('yz')).toEqual({ u: 'y', v: 'z', normal: 'x' });
		expect(previewPlaneAxes('xz')).toEqual({ u: 'x', v: 'z', normal: 'y' });
	});
	it('preserves physical proportions for wide, tall and one-cell planes', () => {
		const wide = fitPreviewPlane(600, 400, 120, 30);
		expect(wide.width / wide.height).toBe(4);
		expect(wide.height).toBe(150);
		expect(wide.top).toBe(125);
		const tall = fitPreviewPlane(600, 400, 20, 80);
		expect(tall.width / tall.height).toBe(0.25);
		expect(tall.left).toBe(250);
		expect(fitPreviewPlane(300, 300, 0, 0)).toEqual({ width: 300, height: 300, left: 0, top: 0 });
	});
});
