import { Buffer } from 'node:buffer';
import { encode } from '@msgpack/msgpack';
import type { Route, WebSocketRoute } from '@playwright/test';
import { expect, test } from './fixtures';

type PreviewMutation = {
	requestId: string;
	quantity?: string;
	component?: string;
	allLayers?: boolean;
};

test('surface quantity and visible progressive view transitions', async ({ page, baseURL }) => {
	let previewSocket: WebSocketRoute | undefined;
	let mainSocket: WebSocketRoute | undefined;
	let sequence = 1;
	let configRevision = 0;
	let component = '3D';
	let quantity = 'm';
	let allLayers = true;
	const errors: string[] = [];
	page.on('pageerror', (error) => errors.push(error.message));

	function sendPreviewFrame(type: '2D' | '3D' = '3D') {
		if (!previewSocket) throw new Error('Preview WebSocket is not connected');
		const scalar = quantity === 'alpha';
		const threeDimensional = type === '3D';
		const vectorValues = new Float32Array(
			Array.from({ length: 24 }, (_, index) => (index % 3 === 0 ? 1 : 0))
		);
		const vectorPositions = new Int32Array(
			Array.from({ length: 24 }, (_, index) =>
				index % 3 === 0
					? Math.floor(index / 3) % 2
					: index % 3 === 1
						? Math.floor(index / 6) % 2
						: Math.floor(index / 12)
			)
		);
		const frame = {
			sequence: sequence++,
			configRevision,
			timestamp: Date.now(),
			quantity,
			component,
			nComp: scalar ? 1 : 3,
			type,
			allLayers,
			unit: '',
			normScale: scalar ? 0.02 : 1,
			vectorCount: threeDimensional ? 8 : 0,
			topologyRevision: sequence,
			appliedXChosenSize: threeDimensional ? 2 : 512,
			appliedYChosenSize: threeDimensional ? 2 : 512,
			appliedZChosenSize: 2,
			appliedLayerStride: 1,
			xChosenSize: 2,
			yChosenSize: 2,
			zChosenSize: 2,
			xPossibleSizes: [1, 2],
			yPossibleSizes: [1, 2],
			zPossibleSizes: [1, 2],
			maxPoints: 1000000,
			dataPointsCount: threeDimensional ? 8 : 262144,
			autoScaleEnabled: false,
			autoDownscaled: false,
			autoDownscaleMessage: '',
			plane: 'xy',
			planeUChosenSize: 512,
			planeVChosenSize: 512,
			appliedPlaneUSize: 512,
			appliedPlaneVSize: 512,
			planeUPossibleSizes: [512],
			planeVPossibleSizes: [512],
			min: 0.02,
			max: 0.02,
			scalarField: threeDimensional
				? []
				: Array.from({ length: 262144 }, (_, index) => [
						index % 512,
						Math.floor(index / 512),
						0.02
					]),
			vectorValuesBinary: new Uint8Array(vectorValues.buffer),
			vectorPositionsBinary: new Uint8Array(vectorPositions.buffer)
		};
		previewSocket.send(Buffer.from(encode(frame)));
	}

	async function waitForTransitionAck(revision: number) {
		await expect
			.poll(
				() =>
					page.evaluate(async (expectedRevision) => {
						const transitionPath = '/src/api/previewTransition.ts';
						const { previewTransition } = await import(transitionPath);
						const snapshot: { value: { expectedRevision?: number } | null } = { value: null };
						const unsubscribe = previewTransition.subscribe(
							(value: { expectedRevision?: number } | null) => (snapshot.value = value)
						);
						unsubscribe();
						return snapshot.value?.expectedRevision === expectedRevision;
					}, revision),
				{ timeout: 15000 }
			)
			.toBe(true);
	}

	await page.routeWebSocket(/\/ws\/preview$/, (socket) => {
		previewSocket = socket;
	});
	await page.routeWebSocket(/\/ws$/, (socket) => {
		mainSocket = socket;
	});
	await page.route(`${baseURL}/api/preview/quantity`, async (route: Route) => {
		await new Promise((resolve) => setTimeout(resolve, 450));
		const request = route.request().postDataJSON() as PreviewMutation;
		quantity = request.quantity ?? quantity;
		configRevision++;
		sendPreviewFrame();
		await route.fulfill({ json: { requestId: request.requestId, configRevision } });
	});
	await page.route(`${baseURL}/api/preview/component`, async (route: Route) => {
		await new Promise((resolve) => setTimeout(resolve, 450));
		const request = route.request().postDataJSON() as PreviewMutation;
		component = request.component ?? component;
		if (request.allLayers !== undefined) allLayers = request.allLayers;
		else if (component === '3D') allLayers = true;
		configRevision++;
		await route.fulfill({ json: { requestId: request.requestId, configRevision } });
		await waitForTransitionAck(configRevision);
		sendPreviewFrame(component === '3D' ? '3D' : '2D');
	});
	await page.route(`${baseURL}/api/preview/allLayers`, async (route: Route) => {
		const request = route.request().postDataJSON() as PreviewMutation;
		allLayers = request.allLayers ?? allLayers;
		configRevision++;
		sendPreviewFrame();
		await route.fulfill({ json: { requestId: request.requestId, configRevision } });
	});

	await page.goto('/');
	await expect.poll(() => Boolean(previewSocket && mainSocket)).toBe(true);
	if (!mainSocket) throw new Error('Main WebSocket is not connected');
	mainSocket.send(
		Buffer.from(encode({ mesh: { Nx: 512, Ny: 512, Nz: 2, dx: 1e-9, dy: 1e-9, dz: 1e-9 } }))
	);
	sendPreviewFrame();

	await page.getByRole('button', { name: 'Volume', exact: true }).click();
	await page.getByLabel('Color by', { exact: true }).selectOption('geometry');
	await expect(page.getByLabel('Color by', { exact: true })).toHaveValue('geometry');
	await page.getByLabel('Quantity', { exact: true }).selectOption('alpha');
	await expect(page.getByRole('status').filter({ hasText: 'Loading alpha' })).toBeVisible();
	await expect(page.locator('#container')).toHaveAttribute('aria-busy', 'false');
	await expect(page.getByLabel('Color by', { exact: true })).toHaveValue('value');

	await page.evaluate(() => {
		const browserWindow = window as unknown as Window & {
			renderGaps: number[];
			renderHeartbeat: number;
		};
		browserWindow.renderGaps = [];
		let previous = performance.now();
		browserWindow.renderHeartbeat = window.setInterval(() => {
			const now = performance.now();
			browserWindow.renderGaps.push(now - previous);
			previous = now;
		}, 16);
	});
	await page.evaluate(() => {
		const browserWindow = window as unknown as Window & {
			renderingStatusObserved: boolean;
			renderingStatusObserver: MutationObserver;
		};
		browserWindow.renderingStatusObserved = false;
		browserWindow.renderingStatusObserver = new MutationObserver(() => {
			browserWindow.renderingStatusObserved ||= Array.from(
				document.querySelectorAll<HTMLElement>('[role="status"]')
			).some((status) => status.textContent?.includes('Rendering the new view'));
		});
		browserWindow.renderingStatusObserver.observe(document.body, {
			childList: true,
			characterData: true,
			subtree: true
		});
	});
	const start = Date.now();
	await page.getByRole('button', { name: '2D section', exact: true }).click();
	await expect(
		page.getByRole('status').filter({ hasText: 'Switching to 2D section' })
	).toBeVisible();
	await expect(page.locator('#container')).toHaveAttribute('aria-busy', 'false', {
		timeout: 20000
	});
	const renderingStatusObserved = await page.evaluate(() => {
		const browserWindow = window as unknown as Window & {
			renderingStatusObserved: boolean;
			renderingStatusObserver: MutationObserver;
		};
		browserWindow.renderingStatusObserver.disconnect();
		return browserWindow.renderingStatusObserved;
	});
	expect(renderingStatusObserved).toBe(true);
	const switchMs = Date.now() - start;
	expect(switchMs).toBeLessThan(20000);
	const maxUiGap = await page.evaluate(() => {
		const browserWindow = window as unknown as Window & {
			renderGaps: number[];
			renderHeartbeat: number;
		};
		clearInterval(browserWindow.renderHeartbeat);
		return Math.max(...browserWindow.renderGaps);
	});
	expect(maxUiGap).toBeLessThan(1000);

	await page.getByRole('button', { name: '3D volume', exact: true }).click();
	await expect(
		page.getByRole('status').filter({ hasText: 'Switching to 3D volume' })
	).toBeVisible();
	await expect(page.locator('#container')).toHaveAttribute('aria-busy', 'false');
	expect(errors).toEqual([]);
	console.log(JSON.stringify({ largePlaneCells: 262144, switchMs, maxUiGap }));
});
