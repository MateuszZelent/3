import { describe, expect, it } from 'vitest';
import { analyzeCore, getCorePlotCounts, sampleCoreCoordinates } from './analysis';

describe('core trajectory validity', () => {
	it('excludes invalid rows and breaks analytics at gaps', () => {
		const result = analyzeCore(
			[
				[0, 1e-9, 0, 1],
				[1e-9, NaN, 0],
				[2e-9, 0, 0, 0],
				[3e-9, 2e-9, 3e-9, 1]
			],
			10
		);
		expect(result.valid).toHaveLength(2);
		expect(result.recent).toHaveLength(1);
		expect(result.metrics.frequencyGHz).toBeNull();
		expect(result.metrics.centerXNm).toBe(2);
		expect(analyzeCore([[0, 0, 0, 0]], 2).metrics.hasData).toBe(false);
	});
	it('reports a sampled complete orbit and rejects drift, stationary and partial data', () => {
		const orbit = Array.from({ length: 101 }, (_, i) => [
			i * 1e-11,
			1e-9 * Math.cos((i * 2 * Math.PI) / 100),
			1e-9 * Math.sin((i * 2 * Math.PI) / 100),
			1
		]);
		expect(analyzeCore(orbit, 2).metrics.frequencyGHz).toBeCloseTo(1, 2);
		expect(analyzeCore(orbit.slice(0, 25), 2).metrics.frequencyGHz).toBeNull();
		expect(
			analyzeCore(
				orbit.map((r) => [r[0], 0, 0, 1]),
				2
			).metrics.frequencyGHz
		).toBeNull();
		expect(
			analyzeCore(
				orbit.map((r) => [r[0], r[0], 0, 1]),
				2
			).metrics.frequencyGHz
		).toBeNull();
	});
	it('breaks at time resets and rejects undersampling', () => {
		expect(
			analyzeCore(
				[
					[0, 1, 2],
					[0, 2, 3],
					[1e-9, 3, 4]
				],
				2
			).recent
		).toHaveLength(2);
		const coarse = Array.from({ length: 9 }, (_, i) => [
			i * 1e-10,
			Math.cos(i * Math.PI),
			Math.sin(i * Math.PI),
			1
		]);
		expect(analyzeCore(coarse, 2).metrics.frequencyGHz).toBeNull();
	});

	it('bounds rendered coordinates at large track sizes while retaining both endpoints', () => {
		const rows = Array.from({ length: 100_001 }, (_, index) => [index, index, -index]);

		const points = sampleCoreCoordinates(rows, 0, rows.length, 5_000);

		expect(points).toHaveLength(5_000);
		expect(points[0][0]).toBe(0);
		expect(points[0][1]).toBeCloseTo(0);
		expect(points.at(-1)).toEqual([100_000, -100_000]);
		for (let index = 1; index < points.length; index++) {
			expect(points[index][0]).toBeGreaterThan(points[index - 1][0]);
		}
	});

	it('reports when rendered core markers are sampled from the valid source records', () => {
		expect(getCorePlotCounts(100_000, 2_000, 5_000)).toEqual({
			sourcePoints: 100_000,
			renderedPoints: 7_000,
			sampled: true
		});
		expect(getCorePlotCounts(123, 23, 5_000)).toEqual({
			sourcePoints: 123,
			renderedPoints: 123,
			sampled: false
		});
	});
});
