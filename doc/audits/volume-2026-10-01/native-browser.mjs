import { chromium } from '../../../frontend/node_modules/playwright/index.mjs';
import { decode } from '../../../frontend/node_modules/@msgpack/msgpack/dist.esm/index.mjs';
import { writeFile } from 'node:fs/promises';
const url = process.env.PREVIEW_URL || 'http://127.0.0.1:35367';
const browser = await chromium.launch({ headless: true }),
	page = await browser.newPage({ viewport: { width: 1640, height: 1200 } }),
	frames = [],
	errors = [];
page.on('pageerror', (e) => errors.push(e.message));
page.on('websocket', (socket) => {
	if (socket.url().endsWith('/preview'))
		socket.on('framereceived', (event) => frames.push(decode(new Uint8Array(event.payload))));
});
const assert = (condition, message) => {
	if (!condition) throw Error(message);
};
async function post(endpoint, data) {
	const r = await page.request.post(url + '/api/preview/' + endpoint, { data });
	assert(r.ok(), endpoint + ': ' + (await r.text()));
}
async function frameWhere(test) {
	for (let i = 0; i < 300; i++) {
		const f = frames.findLast(test);
		if (f) return f;
		await page.waitForTimeout(50);
	}
	throw Error('Frame timeout: ' + JSON.stringify(frames.slice(-1)));
}
try {
	await page.goto(url);
	await page.getByRole('button', { name: 'Volume', exact: true }).click();
	await post('autoScaleEnabled', { autoScaleEnabled: false });
	await post('fullResolution', {});
	const initial = await frameWhere(
		(f) =>
			f.type === '3D' &&
			f.allLayers &&
			f.appliedXChosenSize === 8 &&
			f.appliedYChosenSize === 8 &&
			f.vectorCount === 240
	);
	await page.getByLabel('Color by', { exact: true }).selectOption('z');
	await page.getByRole('button', { name: 'Manual', exact: true }).click();
	await page.getByLabel('Volume color minimum').fill('-1');
	await page.getByLabel('Volume color maximum').fill('1');
	await page.waitForTimeout(500);
	await page.locator('.preview-wrapper').screenshot({ path: '/tmp/3-volume-native-surface.png' });
	await page.getByRole('button', { name: 'Thickness average', exact: true }).click();
	await page.waitForTimeout(500);
	await page
		.locator('.preview-wrapper')
		.screenshot({
			path: '/tmp/3-volume-native-average.png',
			style: '.topbar{visibility:hidden!important}'
		});
	await post('quantity', { quantity: 'B_ext' });
	const global = await frameWhere(
		(f) => f.type === '3D' && f.quantity === 'B_ext' && f.vectorCount === 256
	);
	const occupied = Array.from(global.vectorOccupancy).filter((v) => v > 0).length;
	assert(occupied === 240, 'Nonzero global field must preserve cavity');
	await post('quantity', { quantity: 'geom' });
	const scalar = await frameWhere(
		(f) => f.type === '3D' && f.nComp === 1 && f.quantity === 'geom' && f.vectorCount === 240
	);
	await page.getByLabel('Color by', { exact: true }).selectOption('value');
	assert(
		await page.getByRole('button', { name: 'Arrows', exact: true }).isDisabled(),
		'Scalar arrows disabled'
	);
	await page.locator('.preview-wrapper').screenshot({ path: '/tmp/3-volume-native-scalar.png' });
	await page.getByRole('button', { name: '2D section', exact: true }).click();
	await frameWhere((f) => f.type === '2D' && f.quantity === 'geom');
	await page.getByRole('button', { name: '3D volume', exact: true }).click();
	await frameWhere((f) => f.sequence > scalar.sequence && f.type === '3D' && f.quantity === 'geom');
	await page.setViewportSize({ width: 390, height: 800 });
	await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
	const canvas = await page.locator('#container').boundingBox();
	assert(canvas.height > 200, 'Mobile fullscreen chart height');
	await page.locator('.preview-wrapper').screenshot({ path: '/tmp/3-volume-native-mobile.png' });
	await page.getByRole('button', { name: 'Exit fullscreen', exact: true }).click();
	assert(errors.length === 0, errors.join('\n'));
	await writeFile(
		new URL('native-browser.json', import.meta.url),
		JSON.stringify(
			{
				mesh: [8, 8, 4],
				occupied: initial.vectorCount,
				globalField: { points: global.vectorCount, occupied },
				scalar: { quantity: scalar.quantity, points: scalar.vectorCount, nComp: scalar.nComp },
				mobileCanvas: canvas,
				errors
			},
			null,
			2
		)
	);
} finally {
	await browser.close();
}
