import { describe, expect, it } from 'vitest';
import { analyzeCore } from './analysis';

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
});
