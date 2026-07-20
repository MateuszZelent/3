import { describe, expect, it } from 'vitest';
import { previewSampleCoordinateNm } from './preview2DCoordinates';

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
