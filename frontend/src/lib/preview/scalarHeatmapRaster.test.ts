import { describe, expect, it } from 'vitest';
import { buildScalarHeatmapRaster } from './scalarHeatmapRaster';

const palette = ['#000000', '#ffffff'];

describe('scalar preview raster', () => {
	it('keeps exact coordinate lookups and transparent holes', () => {
		const points = [
			[0, 0, 0],
			[2, 1, 1]
		] as const;
		const raster = buildScalarHeatmapRaster(points, 3, 2, 0, 1, palette);
		expect(raster.getSourceIndex(0, 0)).toBe(0);
		expect(raster.getSourceIndex(2, 1)).toBe(1);
		expect(raster.getSourceIndex(1, 0)).toBe(-1);
		expect(raster.pixels[(1 * 3 + 0) * 4 + 3]).toBe(255);
		expect(raster.pixels[(1 * 3 + 1) * 4 + 3]).toBe(0);
	});

	it('preserves input-order overwrite semantics for duplicate and unsorted points', () => {
		const raster = buildScalarHeatmapRaster(
			[
				[1, 0, 0],
				[0, 0, 1],
				[1, 0, 1]
			],
			2,
			1,
			0,
			1,
			palette
		);
		expect(raster.getSourceIndex(1, 0)).toBe(2);
		expect(raster.getSourceIndex(0, 0)).toBe(1);
		expect(Array.from(raster.pixels.slice(4, 8))).toEqual([255, 255, 255, 255]);
	});

	it('limits raster memory to viewport pixels while retaining every sparse coordinate', () => {
		const raster = buildScalarHeatmapRaster(
			[
				[100, 100, 1],
				[900, 900, 0]
			],
			5000,
			5000,
			0,
			1,
			palette,
			{ viewportWidth: 80, viewportHeight: 60, pixelRatio: 1 }
		);
		expect(raster.width).toBe(80);
		expect(raster.height).toBe(60);
		expect(raster.getSourceIndex(100, 100)).toBe(0);
		expect(raster.getSourceIndex(900, 900)).toBe(1);
		expect(raster.getSourceIndex(1000, 1000)).toBe(-1);
	});

	it('uses the requested complete data window and rejects invalid coordinates', () => {
		const raster = buildScalarHeatmapRaster(
			[
				[-1, 0, 1],
				[0, 0, 1],
				[3, 1, 0],
				[1, 1, 0]
			],
			4,
			2,
			0,
			1,
			palette,
			{ window: { u: [0.25, 0.75], v: [0, 1] } }
		);
		expect(raster.getSourceIndex(-1, 0)).toBe(-1);
		expect(raster.getSourceIndex(3, 1)).toBe(2);
		expect(raster.pixels[(1 * 4 + 0) * 4 + 3]).toBe(0);
	});

	it('fills zoom-expanded dense cells without filling real sparse holes', () => {
		const points = [
			[2, 0, 1],
			[3, 0, 1],
			[5, 0, 1]
		] as const;
		const raster = buildScalarHeatmapRaster(points, 8, 1, 0, 1, palette, {
			window: { u: [0.25, 0.75], v: [0, 1] },
			viewportWidth: 8,
			viewportHeight: 1,
			pixelRatio: 1
		});

		expect(Array.from({ length: 8 }, (_, x) => raster.pixels[x * 4 + 3])).toEqual([
			255, 255, 255, 255, 0, 0, 255, 255
		]);
		expect(Array.from(raster.pixels.slice(0, 4))).toEqual([255, 255, 255, 255]);
		expect(Array.from(raster.pixels.slice(16, 20))).toEqual([0, 0, 0, 0]);
		expect(raster.getSourceIndex(2, 0)).toBe(0);
		expect(raster.getSourceIndex(4, 0)).toBe(-1);
	});
});
