import { expect, test } from './fixtures';
import type { Preview } from '../src/api/incoming/preview';

test('2D axes, physical aspect, editable slices and mobile layout', async ({ page }) => {
	let configRevision = 0;
	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));
	await page.routeWebSocket(/\/ws/, () => {});
	await page.route('**/api/preview/section', async (route) => {
		const request = route.request().postDataJSON() as {
			requestId: string;
			plane?: 'xy' | 'yz' | 'xz';
			sliceIndex?: number;
			mode?: 'single' | 'average';
		};
		const revision = ++configRevision;
		await page.evaluate(
			async ({ request, revision }) => {
				const statePath = '/src/api/incoming/preview.ts';
				const { previewState } = await import(statePath);
				previewState.update((state: Preview) => {
					const plane = request.plane || state.plane || 'xy';
					const dimensions: Record<'xy' | 'yz' | 'xz', [number, number]> = {
						xy: [7, 5],
						yz: [5, 3],
						xz: [7, 3]
					};
					const [u, v] = dimensions[plane];
					return {
						...state,
						configRevision: revision,
						sequence: (state.sequence ?? 0) + 1,
						plane,
						sliceIndex: request.sliceIndex ?? state.sliceIndex,
						allLayers: request.mode ? request.mode === 'average' : state.allLayers,
						planeUChosenSize: u,
						planeVChosenSize: v,
						appliedPlaneUSize: u,
						appliedPlaneVSize: v,
						planeUPossibleSizes: Array.from({ length: u }, (_, i) => i + 1),
						planeVPossibleSizes: Array.from({ length: v }, (_, i) => i + 1),
						scalarField: Array.from({ length: u * v }, (_, i) => [
							i % u,
							Math.floor(i / u),
							(i % u) - 2
						])
					};
				});
				const path = '/src/lib/preview/preview2D.ts';
				const { preview2D } = await import(path);
				await preview2D();
			},
			{ request, revision }
		);
		await route.fulfill({ json: { requestId: request.requestId, configRevision: revision } });
	});
	await page.goto('/');
	await page.getByRole('button', { name: '2D section', exact: true }).waitFor();
	await page.evaluate(async () => {
		const mPath = '/src/api/incoming/mesh.ts',
			pPath = '/src/api/incoming/preview.ts',
			rPath = '/src/lib/preview/preview2D.ts';
		const { meshState } = await import(mPath),
			{ previewState } = await import(pPath);
		meshState.set({ Nx: 7, Ny: 5, Nz: 3, dx: 5e-9, dy: 11e-9, dz: 17e-9 });
		previewState.update((state: Preview) => ({
			...state,
			quantity: 'm',
			nComp: 3,
			type: '2D',
			component: 'x',
			plane: 'xy',
			sliceIndex: 0,
			planeUChosenSize: 7,
			planeVChosenSize: 5,
			appliedPlaneUSize: 7,
			appliedPlaneVSize: 5,
			planeUPossibleSizes: [1, 2, 3, 4, 5, 6, 7],
			planeVPossibleSizes: [1, 2, 3, 4, 5],
			scalarField: Array.from({ length: 35 }, (_, i) => [i % 7, Math.floor(i / 7), (i % 7) - 3]),
			min: -3,
			max: 3,
			dataPointsCount: 35
		}));
		const { preview2D, setPreview2DAutoscale } = await import(rPath);
		setPreview2DAutoscale(false);
		await preview2D();
	});
	for (const [plane, axisX, axisY, ratio, maxSlice] of [
		['XY', 'x', 'y', 35 / 55, 2],
		['YZ', 'y', 'z', 55 / 51, 6],
		['XZ', 'x', 'z', 35 / 51, 4]
	] as const) {
		await page.getByRole('button', { name: plane, exact: true }).click();
		const readChart = () =>
			page.evaluate(async () => {
				const path = '/node_modules/.vite/deps/echarts_core.js';
				const { getInstanceByDom } = await import(path);
				const chart = getInstanceByDom(document.getElementById('container'));
				const option = chart.getOption();
				return {
					axisX: option.xAxis[0].name,
					axisY: option.yAxis[0].name,
					width: option.grid[0].width,
					height: option.grid[0].height
				};
			});
		await expect.poll(readChart).toMatchObject({ axisX: `${axisX} (nm)`, axisY: `${axisY} (nm)` });
		const chart = await readChart();
		expect(chart.width / chart.height).toBeCloseTo(ratio);
		await page.getByLabel('Slice index', { exact: true }).fill(String(maxSlice));
		await page.getByLabel('Slice index', { exact: true }).press('Tab');
		await expect(page.getByLabel('Slice index', { exact: true })).toHaveValue(String(maxSlice));
		await expect(page.getByRole('button', { name: 'Next layer', exact: true })).toBeDisabled();
		await page.getByRole('button', { name: 'Average', exact: true }).click();
		await expect(page.getByLabel('Slice index', { exact: true })).toHaveCount(0);
		await expect(page.getByText('Arithmetic mean across all', { exact: false })).toBeVisible();
		await page.getByRole('button', { name: 'Single layer', exact: true }).click();
	}
	for (const width of [1640, 1024, 768, 390]) {
		await page.setViewportSize({ width, height: 1000 });
		const layout = await page.evaluate(() => ({
			width: document.documentElement.clientWidth,
			scroll: document.documentElement.scrollWidth,
			wide: Array.from(document.querySelectorAll('body *'))
				.filter((el) => el.getBoundingClientRect().right > window.innerWidth + 1)
				.slice(0, 15)
				.map((el) => ({
					class: el.className,
					width: el.getBoundingClientRect().width,
					scroll: el.scrollWidth
				}))
		}));
		expect(layout.scroll, JSON.stringify(layout)).toBeLessThanOrEqual(layout.width + 1);
	}
	expect(errors).toEqual([]);
});
