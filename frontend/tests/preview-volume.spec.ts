import { expect, test } from '@playwright/test';

test('volume sampling exposes Z and preserves a cube at unequal preview sizes', async ({
	page
}) => {
	const errors: string[] = [];
	page.on('pageerror', (error) => errors.push(error.message));
	await page.route('**/api/preview/*', async (route) => {
		const data = route.request().postDataJSON();
		await page.evaluate(async (data) => {
			const path = '/src/api/incoming/preview.ts';
			const { previewState } = await import(path);
			previewState.update((state: object) => ({ ...state, ...data, refresh: true }));
		}, data);
		await route.fulfill({ json: null });
	});
	await page.goto('/');
	await expect(page.getByRole('button', { name: 'Single layer', exact: true })).toBeVisible();
	await page.evaluate(async () => {
		const meshPath = '/src/api/incoming/mesh.ts';
		const previewPath = '/src/api/incoming/preview.ts';
		const rendererPath = '/src/lib/preview/preview3D.ts';
		const { meshState } = await import(meshPath);
		const { previewState } = await import(previewPath);
		meshState.set({ Nx: 250, Ny: 250, Nz: 250, dx: 5e-9, dy: 5e-9, dz: 5e-9 });
		const values: number[] = [],
			positions: number[] = [];
		for (let z = 12; z < 250; z += 25) {
			for (let y = 0; y < 50; y++) {
				for (let x = 0; x < 10; x++) {
					positions.push(x, y, z);
					values.push(0, 1, 0);
				}
			}
		}
		previewState.update((state: object) => ({
			...state,
			quantity: 'm',
			type: '3D',
			nComp: 3,
			component: '3D',
			allLayers: false,
			xPossibleSizes: [1, 2, 5, 10, 25, 50, 125, 250],
			yPossibleSizes: [1, 2, 5, 10, 25, 50, 125, 250],
			zPossibleSizes: [1, 2, 5, 10, 25, 50, 125, 250],
			xChosenSize: 10,
			yChosenSize: 50,
			zChosenSize: 250,
			appliedXChosenSize: 10,
			appliedYChosenSize: 50,
			appliedLayerStride: 25,
			vectorFieldValues: new Float32Array(values),
			vectorFieldPositions: new Int32Array(positions),
			vectorCount: values.length / 3,
			refresh: true
		}));
		await new Promise(requestAnimationFrame);
		const { preview3D } = await import(rendererPath);
		preview3D();
	});
	const zSlider = page.locator('label.slider-field').filter({ hasText: 'Z data points' });
	await expect(zSlider).toHaveCount(0);
	await page.getByRole('button', { name: 'All layers', exact: true }).click();
	await expect(zSlider).toBeVisible();
	await expect(page.getByRole('button', { name: 'All layers', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	const request = page.waitForRequest((req) => req.url().endsWith('/api/preview/ZChosenSize'));
	await zSlider.locator('input').evaluate((element: HTMLInputElement) => {
		element.value = '3';
		element.dispatchEvent(new Event('input', { bubbles: true }));
	});
	// A new server frame must not overwrite an in-progress drag.
	await page.evaluate(async () => {
		const path = '/src/api/incoming/preview.ts';
		const { previewState } = await import(path);
		previewState.update((state: any) => ({ ...state, zPossibleSizes: [...state.zPossibleSizes] }));
	});
	await expect(zSlider.locator('input')).toHaveValue('3');
	await zSlider.locator('input').dispatchEvent('change');
	expect((await request).postDataJSON()).toEqual({ zChosenSize: 10 });
	await expect(zSlider.locator('strong')).toHaveText('10');
	const geometry = await page.evaluate(async () => {
		const path = '/src/lib/preview/preview3D.ts';
		const { preview3D, threeDPreview, setRenderMode } = await import(path);
		preview3D();
		let display: any;
		const unsubscribe = threeDPreview.subscribe((value: any) => (display = value));
		unsubscribe();
		const arrowCount = display.mesh.count;
		const target = display.controls.target.toArray();
		const matrices = display.mesh.instanceMatrix.array;
		const first = Array.from(matrices.slice(12, 15));
		const last = Array.from(matrices.slice((arrowCount - 1) * 16 + 12, (arrowCount - 1) * 16 + 15));
		setRenderMode('voxel');
		return { arrowCount, voxelCount: display.mesh.count, target, first, last };
	});
	expect(geometry.arrowCount).toBe(5000);
	expect(geometry.first).toEqual([5, 5, 1]);
	expect(geometry.last).toEqual([95, 95, 99]);
	expect(geometry.voxelCount).toBeGreaterThan(0);
	for (const coordinate of geometry.target) expect(coordinate).toBeCloseTo(50);
	await page.setViewportSize({ width: 1641, height: 1000 });
	await page.locator('.preview-toolbar').screenshot({ path: '/tmp/preview-toolbar-desktop.png' });
	for (const width of [1024, 768, 390]) {
		await page.setViewportSize({ width, height: 1000 });
		await expect(zSlider).toBeVisible();
		const fits = await page
			.locator('.preview-toolbar')
			.evaluate((element) => element.scrollWidth <= element.clientWidth + 1);
		expect(fits).toBe(true);
	}
	await page.evaluate(async () => {
		const path = '/src/api/incoming/preview.ts';
		const { previewState } = await import(path);
		previewState.update((state: object) => ({
			...state,
			zPossibleSizes: undefined,
			zChosenSize: undefined
		}));
	});
	await expect(zSlider.locator('input')).toBeDisabled();
	await expect(page.getByText('Z resolution is unavailable', { exact: false })).toBeVisible();
	await page.getByRole('button', { name: 'Single layer', exact: true }).click();
	await expect(zSlider).toHaveCount(0);
	await expect(page.locator('label.slider-field').filter({ hasText: 'Z layer' })).toBeVisible();
	expect(errors).toEqual([]);
});
