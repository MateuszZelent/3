import { chromium } from '../../../frontend/node_modules/playwright/index.mjs';
import { decode } from '../../../frontend/node_modules/@msgpack/msgpack/dist.esm/index.mjs';
import { writeFile } from 'node:fs/promises';
const url = process.env.PREVIEW_URL || 'http://127.0.0.1:35367';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1640, height: 1200 } });
const frames = [],
	errors = [],
	checks = [],
	responsive = [];
page.on('pageerror', (e) => errors.push(e.message));
page.on('console', (msg) => {
	if (msg.type() === 'error') errors.push(msg.text());
});
page.on('websocket', (socket) => {
	if (!socket.url().endsWith('/preview')) return;
	socket.on('framereceived', (event) => {
		try {
			frames.push(decode(new Uint8Array(event.payload)));
		} catch (e) {
			errors.push(String(e));
		}
	});
});
const size = [7, 5, 3],
	cells = [5, 11, 17];
const axes = { xy: [0, 1, 2], yz: [1, 2, 0], xz: [0, 2, 1] };
async function post(endpoint, data) {
	const response = await page.request.post(url + '/api/preview/' + endpoint, { data });
	if (!response.ok()) throw Error(endpoint + ': ' + (await response.text()));
}
async function frameAfter(sequence, predicate) {
	const end = Date.now() + 15000;
	while (Date.now() < end) {
		const frame = frames.find((f) => f.sequence > sequence && predicate(f));
		if (frame) return frame;
		await new Promise((r) => setTimeout(r, 30));
	}
	throw Error(
		'Frame timeout: ' +
			JSON.stringify(
				frames
					.slice(-2)
					.map(
						({
							plane,
							type,
							sliceIndex,
							allLayers,
							appliedPlaneUSize,
							appliedPlaneVSize,
							sequence
						}) => ({
							plane,
							type,
							sliceIndex,
							allLayers,
							appliedPlaneUSize,
							appliedPlaneVSize,
							sequence
						})
					)
			)
	);
}
function verify(frame) {
	const [u, v, n] = axes[frame.plane];
	const du = frame.appliedPlaneUSize,
		dv = frame.appliedPlaneVSize;
	if (frame.scalarField.length !== du * dv) throw Error('Missing occupied zero or scalar pixel');
	let maxError = 0;
	for (const [i, j, actual] of frame.scalarField) {
		let sum = 0,
			weight = 0;
		for (let z = 0; z < 3; z++)
			for (let y = 0; y < 5; y++)
				for (let x = 0; x < 7; x++) {
					const c = [x, y, z];
					if (!frame.allLayers && c[n] !== frame.sliceIndex) continue;
					const coverage =
						Math.max(
							0,
							Math.min(((i + 1) * size[u]) / du, c[u] + 1) - Math.max((i * size[u]) / du, c[u])
						) *
						Math.max(
							0,
							Math.min(((j + 1) * size[v]) / dv, c[v] + 1) - Math.max((j * size[v]) / dv, c[v])
						);
					const raw = (x - 3) * 100 + (y - 2) * 10 + z - 1;
					sum += (coverage * raw) / Math.hypot(raw, 500);
					weight += coverage;
				}
		maxError = Math.max(maxError, Math.abs(actual - sum / weight));
	}
	if (maxError > 3e-7) throw Error('Incorrect ' + frame.plane + ' value, max error ' + maxError);
	checks.push({
		plane: frame.plane,
		mode: frame.allLayers ? 'average' : 'single',
		slice: frame.sliceIndex,
		grid: [du, dv],
		maxError
	});
	console.log(JSON.stringify(checks.at(-1)));
}
try {
	await page.goto(url);
	await page.getByRole('button', { name: '2D section', exact: true }).waitFor();
	await page.getByRole('button', { name: '2D section', exact: true }).click();
	await frameAfter(0, (f) => f.type === '2D');
	await post('autoScaleEnabled', { autoScaleEnabled: false });
	await post('fullResolution', {});
	for (const plane of Object.keys(axes)) {
		const [u, v, n] = axes[plane];
		let seq = frames.at(-1)?.sequence || 0;
		await page.getByRole('button', { name: plane.toUpperCase(), exact: true }).click();
		await post('section', { plane, mode: 'single', sliceIndex: 0 });
		verify(
			await frameAfter(
				seq,
				(f) =>
					f.plane === plane &&
					!f.allLayers &&
					f.sliceIndex === 0 &&
					f.appliedPlaneUSize === size[u] &&
					f.appliedPlaneVSize === size[v]
			)
		);
		seq = frames.at(-1).sequence;
		await page.getByLabel('Slice index', { exact: true }).fill(String(size[n] - 1));
		await page.getByLabel('Slice index', { exact: true }).press('Tab');
		verify(
			await frameAfter(
				seq,
				(f) => f.plane === plane && !f.allLayers && f.sliceIndex === size[n] - 1
			)
		);
		seq = frames.at(-1).sequence;
		await page.getByRole('button', { name: 'Average', exact: true }).click();
		verify(await frameAfter(seq, (f) => f.plane === plane && f.allLayers));
		seq = frames.at(-1).sequence;
		await post('planeResolution', { uSize: 3, vSize: 2 });
		verify(
			await frameAfter(
				seq,
				(f) =>
					f.plane === plane && f.allLayers && f.appliedPlaneUSize === 3 && f.appliedPlaneVSize === 2
			)
		);
		await post('fullResolution', {});
	}
	for (const data of [
		{ plane: 'zx' },
		{ mode: 'maximum' },
		{ plane: 'yz', sliceIndex: 7 },
		{ plane: 'xz', sliceIndex: -1 },
		{ plane: 'xy', sliceIndex: 1.5 }
	]) {
		const response = await page.request.post(url + '/api/preview/section', { data });
		if (response.status() !== 400) throw Error('Invalid section accepted: ' + JSON.stringify(data));
	}
	let seq = frames.at(-1).sequence;
	await post('section', { plane: 'xy', mode: 'single', sliceIndex: 1 });
	await frameAfter(seq, (f) => f.plane === 'xy' && !f.allLayers && f.sliceIndex === 1);
	for (const width of [1640, 1024, 768, 390]) {
		await page.setViewportSize({ width, height: 1100 });
		await page.evaluate(() => window.scrollTo(0, 0));
		await page.waitForTimeout(350);
		const metrics = await page.locator('[data-panel="preview"]').evaluate((element) => ({
			width: element.clientWidth,
			scrollWidth: element.scrollWidth,
			pageWidth: document.documentElement.clientWidth,
			pageScrollWidth: document.documentElement.scrollWidth
		}));
		if (metrics.scrollWidth > metrics.width + 1 || metrics.pageScrollWidth > metrics.pageWidth + 1)
			throw Error('Horizontal overflow at ' + width + ': ' + JSON.stringify(metrics));
		responsive.push({ viewport: width, ...metrics });
		await page.screenshot({
			path: `/tmp/3-preview-planes-${width}.png`,
			clip: await page.locator('[data-panel="preview"]').boundingBox()
		});
	}
	await page.setViewportSize({ width: 1640, height: 1200 });
	await page.getByRole('button', { name: 'Popout', exact: true }).click();
	await page.locator('.preview-wrapper--popout').waitFor();
	const popoutBox = await page.locator('.preview-wrapper--popout').boundingBox();
	if (Math.abs(popoutBox.x - 60) > 1 || Math.abs(popoutBox.y - 60) > 1)
		throw Error('Incorrect popout position: ' + JSON.stringify(popoutBox));
	await page
		.locator('.floating-section-controls')
		.getByRole('button', { name: 'YZ', exact: true })
		.click();
	await page
		.locator('.preview-wrapper--popout')
		.screenshot({ path: '/tmp/3-preview-planes-popout.png' });
	await page
		.locator('.preview-wrapper__title-actions')
		.getByRole('button', { name: 'Dock', exact: true })
		.click();
	await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
	await page.getByRole('button', { name: 'Exit fullscreen', exact: true }).click();
	await page.getByRole('button', { name: '3D volume', exact: true }).click();
	await page.getByRole('button', { name: 'All layers', exact: true }).click();
	await page.getByRole('button', { name: 'Voxel', exact: true }).click();
	await page
		.locator('.advanced-appearance')
		.getByText('Lighting, clipping & material', { exact: true })
		.click();
	await page.evaluate(() => window.scrollTo(0, 0));
	await page.screenshot({
		path: '/tmp/3-preview-planes-3d.png',
		clip: await page.locator('[data-panel="preview"]').boundingBox()
	});

	const geometryChecks = [];
	const consoleCommand = async (command) => {
		const response = await page.request.post(url + '/api/console/command', { data: { command } });
		if (!response.ok()) throw Error(await response.text());
	};
	await consoleCommand('SetGeom(Cuboid(15e-9,100e-9,100e-9)); m=Uniform(0,1,0)');
	await post('component', { component: 'x' });
	for (const [plane, mode, sliceIndex, count, component, expected] of [
		['xy', 'single', 1, 15, 'x', 0],
		['yz', 'single', 1, 0, 'x', 0],
		['yz', 'single', 3, 15, 'x', 0],
		['yz', 'average', 3, 15, 'x', 0],
		['yz', 'average', 3, 15, 'y', 3 / 7],
		['xz', 'average', 2, 9, 'y', 1]
	]) {
		seq = frames.at(-1).sequence;
		await post('component', { component });
		await post('section', { plane, mode, sliceIndex });
		const frame = await frameAfter(
			seq,
			(f) =>
				f.type === '2D' &&
				f.plane === plane &&
				f.component === component &&
				f.allLayers === (mode === 'average') &&
				f.sliceIndex === sliceIndex &&
				f.dataPointsCount === count
		);
		if ((frame.scalarField || []).some((pixel) => Math.abs(pixel[2] - expected) > 1e-7))
			throw Error('Incorrect geometry reduction');
		geometryChecks.push({ plane, mode, sliceIndex, count, component, expected });
	}
	await consoleCommand('SetGeom(Cuboid(5e-9,100e-9,100e-9).Transl(10e-9,0,0)); m=Uniform(0,1,0)');
	seq = frames.at(-1).sequence;
	await post('section', { plane: 'xz', mode: 'average' });
	const moved = await frameAfter(seq, (f) => f.plane === 'xz' && f.dataPointsCount === 3);
	if (moved.scalarField.some(([x, z, value]) => x !== 5 || value !== 1))
		throw Error('Stale geometry mask after shape change');
	geometryChecks.push({ case: 'changed-geometry', count: 3 });
	await consoleCommand('SetGeom(Cuboid(1e-6,1e-6,1e-6)); SetGridSize(2,2,1); m=Uniform(0,1,0)');
	seq = frames.at(-1).sequence;
	await post('section', { plane: 'yz', mode: 'single' });
	const shrunk = await frameAfter(
		seq,
		(f) =>
			f.plane === 'yz' &&
			f.sliceIndex === 1 &&
			f.appliedPlaneUSize === 2 &&
			f.appliedPlaneVSize === 1
	);
	geometryChecks.push({
		case: 'mesh-shrink-clamps-slice',
		index: shrunk.sliceIndex,
		grid: [shrunk.appliedPlaneUSize, shrunk.appliedPlaneVSize]
	});
	if (errors.length) throw Error(errors.join('\n'));
	await writeFile(
		process.env.PREVIEW_REPORT || new URL('planes-browser.json', import.meta.url),
		JSON.stringify({ checks, geometryChecks, responsive, errors }, null, 2)
	);
} finally {
	await browser.close();
}
