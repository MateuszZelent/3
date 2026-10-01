import { expect, test } from '@playwright/test';

test('long thin 2D samples fill the viewport and ranges pan without resetting on frames', async ({
	page
}) => {
	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));
	page.on('console', (msg) => {
		if (msg.type() === 'error' && !msg.text().includes('WebSocket')) errors.push(msg.text());
	});
	await page.routeWebSocket(/\/ws/, (socket) => socket.close());
	await page.goto('/');
	await page.getByRole('button', { name: '2D section', exact: true }).waitFor();
	await page.evaluate(async () => {
		const meshPath = '/src/api/incoming/mesh.ts',
			statePath = '/src/api/incoming/preview.ts',
			rendererPath = '/src/lib/preview/preview2D.ts',
			wsPath = '/src/api/websocket.ts';
		const { meshState } = await import(meshPath),
			{ previewState } = await import(statePath),
			{ connected, previewConnected } = await import(wsPath);
		connected.set(true);
		previewConnected.set(true);
		meshState.set({ Nx: 4096, Ny: 4, Nz: 2, dx: 10e-9, dy: 10e-9, dz: 10e-9 });
		previewState.update((state: any) => ({
			...state,
			quantity: 'm',
			nComp: 3,
			type: '2D',
			component: 'x',
			plane: 'xy',
			sliceIndex: 0,
			planeUChosenSize: 256,
			planeVChosenSize: 4,
			appliedPlaneUSize: 256,
			appliedPlaneVSize: 4,
			planeUPossibleSizes: [1, 256],
			planeVPossibleSizes: [1, 4],
			scalarField: Array.from({ length: 1024 }, (_, i) => [
				i % 256,
				Math.floor(i / 256),
				(i % 256) / 128 - 1
			]),
			min: -1,
			max: 1,
			dataPointsCount: 1024
		}));
		const { preview2D } = await import(rendererPath);
		preview2D();
	});
	const read = () =>
		page.evaluate(async () => {
			const path = '/node_modules/.vite/deps/echarts_core.js',
				windowPath = '/src/lib/preview/preview2DWindow.ts';
			const { getInstanceByDom } = await import(path),
				{ preview2DWindow } = await import(windowPath);
			const chart = getInstanceByDom(document.getElementById('container')),
				option = chart.getOption();
			let window: any;
			preview2DWindow.subscribe((value: any) => (window = value))();
			const series = chart.getModel().getSeriesByIndex(0),
				data = series.getData();
			const first = data.count() ? data.get('x', 0) : null;
			const last = data.count() ? data.get('x', data.count() - 1) : null;
			return {
				grid: option.grid[0],
				window,
				count: data.count(),
				first,
				last,
				zoom: option.dataZoom
					.filter((item: any) => item.id.startsWith('window-'))
					.map((item: any) => ({ id: item.id, start: item.start, end: item.end }))
			};
		});
	await expect(page.getByLabel('Autoscale', { exact: true })).toBeChecked();
	expect((await read()).grid.height).toBeGreaterThan(100);
	await page.locator('label.ui-toggle').filter({ hasText: 'Autoscale' }).click();
	await expect(page.getByLabel('Autoscale', { exact: true })).not.toBeChecked();
	await expect
		.poll(async () => {
			const { grid } = await read();
			return grid.width / grid.height;
		})
		.toBeCloseTo(1024);
	await page.locator('label.ui-toggle').filter({ hasText: 'Autoscale' }).click();
	// Keyboard editing is accessible; pointer dragging is exercised separately.
	await page.getByLabel('X window end', { exact: true }).fill('0.5');
	await page.getByLabel('X window start', { exact: true }).fill('0.25');
	await page.getByLabel('Y window end', { exact: true }).fill('0.75');
	const before = await read();
	expect(before.window.u[0]).toBeCloseTo(0.25);
	expect(before.window.u[1]).toBeCloseTo(0.5);
	expect(before.count).toBeLessThan(1024);
	expect(before.first).toBeGreaterThan(0);
	expect(before.last).toBeLessThan(255);
	const selection = page.getByRole('slider', { name: 'Move X window', exact: true });
	await selection.scrollIntoViewIfNeeded();
	const box = await selection.boundingBox();
	await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
	await page.mouse.down();
	await page.mouse.move(box!.x + box!.width / 2 + 35, box!.y + box!.height / 2, { steps: 8 });
	await page.mouse.up();
	const moved = await read();
	expect(moved.window.u[0]).toBeGreaterThan(before.window.u[0]);
	expect(moved.window.u[1] - moved.window.u[0]).toBeCloseTo(
		before.window.u[1] - before.window.u[0]
	);
	await page.evaluate(async () => {
		const p = '/src/api/incoming/preview.ts',
			r = '/src/lib/preview/preview2D.ts';
		const { previewState } = await import(p),
			{ preview2D } = await import(r);
		previewState.update((state: any) => ({
			...state,
			sequence: 123,
			scalarField: state.scalarField.map(([x, y, v]: number[]) => [x, y, -v])
		}));
		preview2D();
	});
	expect((await read()).window).toEqual(moved.window);
	await selection.focus();
	await selection.press('ArrowLeft');
	expect((await read()).window.u[0]).toBeLessThan(moved.window.u[0]);
	await page.getByRole('button', { name: 'Full window', exact: true }).click();
	await expect.poll(async () => (await read()).window).toEqual({ u: [0, 1], v: [0, 1] });
	expect((await read()).count).toBe(1024);
	for (const width of [1640, 768, 390]) {
		await page.setViewportSize({ width, height: 1100 });
		await page.locator('.plane-window-controls').scrollIntoViewIfNeeded();
		await page
			.locator('.plane-window-controls')
			.screenshot({ path: `/tmp/3-preview-window-${width}.png` });
		const fits = await page.evaluate(
			() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1
		);
		expect(fits).toBe(true);
	}
	await page.setViewportSize({ width: 1640, height: 1100 });
	await page.getByRole('button', { name: 'Popout', exact: true }).click();
	await expect
		.poll(async () => (await page.locator('.preview-wrapper__canvas').boundingBox())!.height)
		.toBeGreaterThan(250);
	await expect.poll(async () => (await read()).grid.height).toBeGreaterThan(100);
	await page
		.locator('.preview-wrapper--popout')
		.getByRole('button', { name: 'Dock', exact: true })
		.click();
	await page.setViewportSize({ width: 390, height: 1100 });
	await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
	await expect(page.locator('.floating-section')).not.toHaveAttribute('open');
	await expect
		.poll(async () => (await page.locator('.preview-wrapper__canvas').boundingBox())!.height)
		.toBeGreaterThan(250);
	await page.getByLabel('X window end', { exact: true }).fill('0.5');
	expect((await read()).window.u[1]).toBeCloseTo(0.5);
	await page.getByRole('button', { name: 'Exit fullscreen', exact: true }).click();
	expect(errors).toEqual([]);
});
