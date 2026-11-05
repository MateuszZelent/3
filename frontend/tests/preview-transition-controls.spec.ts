import { expect, publishPreviewConfig, test } from './fixtures';
import type { Preview } from '../src/api/incoming/preview';

test('locks the previous 2D plane and resolution controls until the target frame renders', async ({
	page,
	baseURL
}) => {
	const mutations: Array<{ path: string; body: Record<string, unknown> }> = [];
	const pageErrors: string[] = [];
	page.on('pageerror', (error) => pageErrors.push(error.message));
	await page.routeWebSocket(/\/ws(?:\/preview)?$/, () => {});
	await page.route(`${baseURL}/api/**`, async (route) => {
		const url = new URL(route.request().url());
		const body = route.request().postDataJSON() as Record<string, unknown> | null;
		if (url.pathname.endsWith('/api/preview/component') && body) {
			mutations.push({ path: url.pathname, body });
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ requestId: body.requestId, configRevision: 2 })
			});
			return;
		}
		if (url.pathname.endsWith('/api/preview/region') && body) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					enabled: false,
					start: [0, 0, 0],
					end: [8, 8, 8],
					mode: 'native',
					samples: [8, 8, 8],
					maxPoints: 1000000,
					requestId: body.requestId,
					configRevision: 1
				})
			});
			return;
		}
		if (url.pathname.endsWith('/api/demag/progress')) {
			await route.fulfill({
				json: { id: 0, active: false, failed: false, stage: '', percent: 0, startedAt: 0, logs: [] }
			});
			return;
		}
		await route.fulfill({ json: {} });
	});

	await page.goto('/');
	await expect(page.getByRole('button', { name: '3D volume', exact: true })).toBeVisible();
	await page.evaluate(async () => {
		const meshModule: string = '/src/api/incoming/mesh.ts';
		const previewModule: string = '/src/api/incoming/preview.ts';
		const { meshState } = await import(meshModule);
		const { previewState } = await import(previewModule);
		meshState.set({
			Nx: 8,
			Ny: 8,
			Nz: 8,
			dx: 1e-9,
			dy: 1e-9,
			dz: 1e-9,
			Tx: 1,
			Ty: 1,
			Tz: 1,
			PBCx: 0,
			PBCy: 0,
			PBCz: 0
		});
		previewState.update((state: Preview) => ({
			...state,
			type: '2D',
			quantity: 'm',
			component: 'x',
			nComp: 1,
			allLayers: false,
			sequence: 1,
			configRevision: 1,
			plane: 'xy',
			planeUChosenSize: 4,
			planeVChosenSize: 4,
			planeUPossibleSizes: [1, 2, 4, 8],
			planeVPossibleSizes: [1, 2, 4, 8],
			xPossibleSizes: [1, 2, 4, 8],
			yPossibleSizes: [1, 2, 4, 8],
			xChosenSize: 4,
			yChosenSize: 4,
			appliedXChosenSize: 4,
			appliedYChosenSize: 4,
			appliedPlaneUSize: 4,
			appliedPlaneVSize: 4
		}));
	});

	await page.getByRole('button', { name: '3D volume', exact: true }).click();
	await expect.poll(() => mutations.length).toBe(1);
	expect(mutations[0].body).toMatchObject({
		component: '3D',
		allLayers: true,
		requestId: expect.any(String)
	});
	await expect(
		page.getByRole('status').filter({ hasText: 'Switching to 3D volume' })
	).toBeVisible();
	await expect(page.getByRole('button', { name: '2D section', exact: true })).toBeDisabled();
	await expect(page.locator('.plane-button').nth(1)).toBeDisabled();
	const resolutionInputs = page.locator('.resolution-sliders input');
	await expect(resolutionInputs).toHaveCount(2);
	await expect(resolutionInputs.nth(0)).toBeDisabled();
	await expect(resolutionInputs.nth(1)).toBeDisabled();

	// Force DOM events too: outgoing methods independently reject stale controls while busy.
	await page
		.locator('.plane-button')
		.nth(1)
		.evaluate((button: HTMLButtonElement) => button.click());
	await resolutionInputs.nth(0).evaluate((input: HTMLInputElement) => {
		input.value = '1';
		input.dispatchEvent(new Event('input', { bubbles: true }));
		input.dispatchEvent(new Event('change', { bubbles: true }));
	});
	await page.waitForTimeout(150);
	expect(mutations.map((mutation) => mutation.path)).toEqual(['/api/preview/component']);
	await expect(
		page.getByRole('status').filter({ hasText: 'Switching to 3D volume' })
	).toBeVisible();
	expect(pageErrors).toEqual([]);
});

test('accepts the backend-normalized None component when switching a scalar field to 2D', async ({
	page,
	baseURL
}) => {
	const mutations: Array<{ body: Record<string, unknown> }> = [];
	const pageErrors: string[] = [];
	page.on('pageerror', (error) => pageErrors.push(error.message));
	await page.routeWebSocket(/\/ws(?:\/preview)?$/, () => {});
	await page.route(`${baseURL}/api/**`, async (route) => {
		const url = new URL(route.request().url());
		const body = route.request().postDataJSON() as Record<string, unknown> | null;
		if (url.pathname.endsWith('/api/preview/component') && body) {
			mutations.push({ body });
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ requestId: body.requestId, configRevision: 5 })
			});
			return;
		}
		if (url.pathname.endsWith('/api/preview/region') && body) {
			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ requestId: body.requestId, configRevision: 4 })
			});
			return;
		}
		if (url.pathname.endsWith('/api/demag/progress')) {
			await route.fulfill({
				json: { id: 0, active: false, failed: false, stage: '', percent: 0, startedAt: 0, logs: [] }
			});
			return;
		}
		await route.fulfill({ json: {} });
	});

	await page.goto('/');
	await expect(page.getByRole('button', { name: '2D section', exact: true })).toBeVisible();
	await page.evaluate(async () => {
		const previewModule: string = '/src/api/incoming/preview.ts';
		const { previewState } = await import(previewModule);
		previewState.update((state: Preview) => ({
			...state,
			type: '3D',
			quantity: 'geom',
			component: '3D',
			nComp: 1,
			sequence: 7,
			configRevision: 4
		}));
	});

	await page.getByRole('button', { name: '2D section', exact: true }).click();
	await expect.poll(() => mutations.length).toBe(1);
	expect(mutations[0].body).toMatchObject({
		component: 'None',
		requestId: expect.any(String)
	});
	await expect(
		page.getByRole('status').filter({ hasText: 'Switching to 2D section' })
	).toBeVisible();

	await page.evaluate(async () => {
		const meshModule: string = '/src/api/incoming/mesh.ts';
		const { meshState } = await import(meshModule);
		meshState.set({
			Nx: 2,
			Ny: 2,
			Nz: 2,
			dx: 1e-9,
			dy: 1e-9,
			dz: 1e-9,
			Tx: 2,
			Ty: 2,
			Tz: 2,
			PBCx: 0,
			PBCy: 0,
			PBCz: 0
		});
	});
	await publishPreviewConfig(
		page,
		{
			type: '2D',
			quantity: 'geom',
			component: 'None',
			nComp: 1,
			sequence: 8,
			plane: 'xy',
			planeUChosenSize: 2,
			planeVChosenSize: 2,
			appliedPlaneUSize: 2,
			appliedPlaneVSize: 2,
			scalarField: [
				[1, 2],
				[3, 4]
			]
		},
		5
	);
	await expect(page.getByRole('status').filter({ hasText: 'Switching to 2D section' })).toHaveCount(
		0
	);
	await expect(page.getByRole('alert')).toHaveCount(0);
	expect(pageErrors).toEqual([]);
});

test('does not submit the already selected display mode before the first preview frame', async ({
	page,
	baseURL
}) => {
	const mutations: Array<Record<string, unknown>> = [];
	await page.routeWebSocket(/\/ws(?:\/preview)?$/, () => {});
	await page.route(`${baseURL}/api/**`, async (route) => {
		const url = new URL(route.request().url());
		const body = route.request().postDataJSON() as Record<string, unknown> | null;
		if (url.pathname.endsWith('/api/preview/component') && body) mutations.push(body);
		if (url.pathname.endsWith('/api/demag/progress')) {
			await route.fulfill({
				json: { id: 0, active: false, failed: false, stage: '', percent: 0, startedAt: 0, logs: [] }
			});
			return;
		}
		await route.fulfill({ json: {} });
	});

	await page.goto('/');
	const threeDButton = page.getByRole('button', { name: '3D volume', exact: true });
	await expect(threeDButton).toBeVisible();
	await threeDButton.click();
	await page.waitForTimeout(200);
	expect(mutations).toEqual([]);
});
