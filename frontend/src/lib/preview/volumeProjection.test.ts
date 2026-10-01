import { describe, it, expect } from 'vitest';
import { projectVolumeVectors } from './volumeProjection';
import { getColorScale } from './fieldColorScale';
describe('volume surface projection', () => {
	it('weights the final shorter Z group by its actual thickness', () => {
		const p = new Int32Array([0, 0, 1, 0, 0, 2]);
		const values = new Float32Array([1, 0, 0, 0, 0, 0]);
		const result = projectVolumeVectors(p, values, [1, 1, 2], 2, 'z', 3);
		expect(result[0]).toBeCloseTo(2 / 3);
		expect(result[3]).toBeCloseTo(2 / 3);
	});
	it('averages signed values through Z including zero and empty cells', () => {
		const positions = new Int32Array([0, 0, 0, 0, 0, 2, 0, 0, 4, 1, 0, 0]);
		const values = new Float32Array([1, 2, 3, -1, -2, -3, 0, 0, 0, 3, 6, 9]);
		const result = projectVolumeVectors(positions, values, [2, 1, 3], 2, 'z');
		expect(Array.from(result)).toEqual([0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3]);
	});
	it('projects independently along each physical axis', () => {
		const p: number[] = [],
			v: number[] = [];
		for (let z = 0; z < 3; z++)
			for (let y = 0; y < 2; y++)
				for (let x = 0; x < 4; x++) {
					p.push(x, y, z);
					v.push(x, y, z);
				}
		for (const [axis, expected] of [
			['x', [1.5, 0, 0]],
			['y', [0, 0.5, 0]],
			['z', [0, 0, 1]]
		] as const) {
			expect(
				Array.from(
					projectVolumeVectors(new Int32Array(p), new Float32Array(v), [4, 2, 3], 1, axis).slice(
						0,
						3
					)
				)
			).toEqual(expected);
		}
	});
	it('uses the same signed and one-sided field scales as 2D', () => {
		expect(getColorScale(-0.5, 2)).toMatchObject({
			min: -2,
			max: 2,
			palette: ['#15315f', '#2f6caa', '#90b9df', '#f4f1ed', '#efb09d', '#cf6256', '#7d1d34']
		});
		expect(getColorScale(0, 2).palette).toEqual([
			'#0a1220',
			'#143d67',
			'#1c6d8f',
			'#24a0a4',
			'#8ed6ac',
			'#f1f7bb'
		]);
	});
});

it('keeps uniform Float32 fields from expanding rounding noise across the palette', () => {
	const scale = getColorScale(0.01999999955, 0.02000000142);
	expect(scale.min).toBeLessThan(0.0195);
	expect(scale.max).toBeGreaterThan(0.0205);
	const a = (0.01999999955 - scale.min) / (scale.max - scale.min);
	const b = (0.02000000142 - scale.min) / (scale.max - scale.min);
	expect(Math.abs(a - b)).toBeLessThan(1e-5);
	expect(getColorScale(0, 0)).toMatchObject({ min: -1e-12, max: 1e-12 });
});
