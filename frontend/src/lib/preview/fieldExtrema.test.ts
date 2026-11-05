import { describe, expect, it } from 'vitest';
import { getNumericFieldRange } from './fieldExtrema';

describe('numeric preview extrema cache', () => {
	it('keys the cached range by source, component, occupancy, scale, and frame revision', () => {
		const values = new Float32Array([3, -4, 0, 5, 12, 0]);
		const occupied = new Uint8Array([1, 1]);
		const onlyFirst = new Uint8Array([1, 0]);
		const x = getNumericFieldRange(values, 'x', occupied, 2, 2, 7);
		expect(x).toEqual({ min: 6, max: 10 });
		expect(getNumericFieldRange(values, 'x', occupied, 2, 2, 7)).toBe(x);
		expect(getNumericFieldRange(values, 'y', occupied, 2, 2, 7)).toEqual({ min: -8, max: 24 });
		expect(getNumericFieldRange(values, 'x', onlyFirst, 2, 2, 7)).toEqual({ min: 6, max: 6 });
		expect(getNumericFieldRange(values, 'x', occupied, 2, 1, 7)).toEqual({ min: 3, max: 5 });
		expect(getNumericFieldRange(values, 'x', occupied, 2, 2, 8)).not.toBe(x);
	});

	it('ignores empty cells and non-finite values without retaining stale bounds', () => {
		const values = new Float32Array([100, 0, 0, Number.NaN, 0, 0]);
		expect(getNumericFieldRange(values, 'x', new Uint8Array([0, 1]), 2, 1, 1)).toEqual({
			min: 0,
			max: 1
		});
	});
});
