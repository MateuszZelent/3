import { describe, expect, it } from 'vitest';
import { clampWindow, moveWindow } from './preview2DWindow';

describe('2D viewing window', () => {
	it('keeps a minimum span and clamps both boundaries', () => {
		expect(clampWindow([0.4, 0.4], 0.1)).toEqual([0.4, 0.5]);
		expect(clampWindow([-0.2, 0.3])).toEqual([0, 0.5]);
		expect(clampWindow([0.9, 1.4])[0]).toBeCloseTo(0.5);
		expect(clampWindow([0.9, 1.4])[1]).toBe(1);
	});
	it('pans while preserving the width, including at the mesh boundaries', () => {
		expect(moveWindow([0.2, 0.4], 0.1)[0]).toBeCloseTo(0.3);
		expect(moveWindow([0.2, 0.4], 0.1)[1]).toBeCloseTo(0.5);
		expect(moveWindow([0.2, 0.4], -1)[0]).toBe(0);
		expect(moveWindow([0.2, 0.4], -1)[1]).toBeCloseTo(0.2);
		expect(moveWindow([0.2, 0.4], 1)[0]).toBeCloseTo(0.8);
		expect(moveWindow([0.2, 0.4], 1)[1]).toBe(1);
		expect(moveWindow([0, 1], 0.5)).toEqual([0, 1]);
	});
});
