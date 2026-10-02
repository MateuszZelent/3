import { chromium } from '../../../frontend/node_modules/playwright/index.mjs';
import { decode } from '../../../frontend/node_modules/@msgpack/msgpack/dist.esm/index.mjs';
import { writeFile } from 'node:fs/promises';
const url = process.env.PREVIEW_URL || 'http://127.0.0.1:35367';
const browser = await chromium.launch({ headless: true }),
	page = await browser.newPage({ viewport: { width: 1640, height: 1200 } }),
	frames = [],
	errors = [];
page.on('pageerror', (e) => errors.push(e.message));
page.on('websocket', (s) => {
	if (s.url().endsWith('/preview'))
		s.on('framereceived', (e) => frames.push(decode(new Uint8Array(e.payload))));
});
const assert = (c, m) => {
	if (!c) throw Error(m);
};
async function post(endpoint, data) {
	const r = await page.request.post(url + '/api/preview/' + endpoint, { data });
	assert(r.ok(), endpoint + ':' + (await r.text()));
}
async function frameWhere(test) {
	for (let i = 0; i < 300; i++) {
		const f = frames.findLast(test);
		if (f) return f;
		await page.waitForTimeout(50);
	}
	throw Error('Frame timeout:' + JSON.stringify(frames.at(-1)));
}
function verifyNative(f) {
	const positions = new Int32Array(f.vectorPositionsBinary.slice().buffer),
		values = new Float32Array(f.vectorValuesBinary.slice().buffer);
	let occupied = 0;
	for (let i = 0; i < f.vectorCount; i++) {
		const x = f.region.start[0] + positions[i * 3],
			y = f.region.start[1] + positions[i * 3 + 1],
			z = f.region.start[2] + positions[i * 3 + 2];
		assert(
			x >= f.region.start[0] &&
				x < f.region.end[0] &&
				y >= f.region.start[1] &&
				y < f.region.end[1] &&
				z >= f.region.start[2] &&
				z < f.region.end[2],
			'Source window bounds'
		);
		const hole = x >= 2046 && x < 2050 && y >= 3 && y < 5;
		assert(!hole, 'Geometry cavity leaked into native cells');
		const raw = [1, (x - 2050) / 10, y - 3.5],
			norm = Math.hypot(...raw);
		for (let c = 0; c < 3; c++)
			assert(
				Math.abs(values[i * 3 + c] * f.normScale - raw[c] / norm) < 2e-6,
				`Native raw cell mismatch x=${x} y=${y} c=${c}`
			);
		occupied++;
	}
	return occupied;
}
try {
	await page.goto(url);
	await page.getByRole('button', { name: 'Volume', exact: true }).click();
	await post('XChosenSize', { XChosenSize: 100 });
	await post('YChosenSize', { YChosenSize: 8 });
	await post('ZChosenSize', { ZChosenSize: 4 });
	const full = await frameWhere(
		(f) =>
			f.type === '3D' &&
			f.allLayers &&
			f.appliedXChosenSize === 100 &&
			f.appliedYChosenSize === 8 &&
			f.appliedZChosenSize === 4
	);
	assert(await page.getByLabel('X window start', {exact:true}).isVisible(), '3D window sliders hidden by default');
	// Edit exact source-cell bounds through the real UI, not only the HTTP API.
	await page.getByLabel('X first cell', { exact: true }).fill('2040');
	await page.getByLabel('X first cell', { exact: true }).blur();
	await page.getByLabel('X end cell', { exact: true }).fill('2072');
	await page.getByLabel('X end cell', { exact: true }).blur();
	const local = await frameWhere(
		(f) =>
			f.regionActive &&
			f.region.start[0] === 2040 &&
			f.region.end[0] === 2072 &&
			f.appliedXChosenSize === 32 &&
			f.vectorCount === 992
	);
	assert(verifyNative(local) === 992, 'Native geometry point count');
 for(const mode of ['Arrows','Voxel','Volume']) {
 await page.getByRole('button',{name:mode,exact:true}).click();await page.waitForTimeout(250);
 for (const axis of ['X','Y','Z']) assert(await page.getByLabel(axis+' window start',{exact:true}).isVisible(), mode+' window slider hidden');
 assert(await page.getByLabel('Maximum transferred points',{exact:true}).count()===0, 'Local window must receive complete data');
 const stats=await page.locator('.preview-wrapper__stats').textContent();
 assert(stats.includes(mode==='Volume'?'Volume cells: 992':mode==='Voxel'?'Voxels: 992':'Arrows: 992'),`${mode} dropped native window cells: ${stats}`);
 }
	assert(local.xChosenSize === 100 && local.yChosenSize === 8, 'Global resolution overwritten');
	await page.getByRole('button', { name: 'Fit camera', exact: true }).click();
	await page.waitForTimeout(350);
	await page
		.locator('.preview-wrapper')
		.screenshot({
			path: '/tmp/3-region-native.png',
			style: '.topbar{visibility:hidden!important}'
		});
	// Keyboard panning preserves the source-cell width and refreshes source data.
	await page.getByRole('slider', { name: 'Move X window', exact: true }).focus();
	await page.keyboard.press('Shift+ArrowRight');
	const pan = await frameWhere(
		(f) =>
			f.regionActive &&
			f.region.start[0] === 2050 &&
			f.region.end[0] === 2082 &&
			f.vectorCount === 1024
	);
	verifyNative(pan);
	await page.getByRole('button', { name: 'Custom resolution', exact: true }).click();
	await page.getByLabel('Local X samples', { exact: true }).fill('7');
	await page.getByLabel('Local X samples', { exact: true }).blur();
	await page.getByLabel('Local Z samples', { exact: true }).fill('3');
	await page.getByLabel('Local Z samples', { exact: true }).blur();
	const custom = await frameWhere(
		(f) =>
			f.regionActive &&
			f.region.mode === 'custom' &&
			f.appliedXChosenSize === 7 &&
			f.appliedZChosenSize === 3
	);
	assert(custom.appliedLayerStride === 1, 'Uniform local Z bins');
	// Independent analytic CPU reference for cropped fractional X/Z averages.
	const cv = new Float32Array(custom.vectorValuesBinary.slice().buffer),
		cp = new Int32Array(custom.vectorPositionsBinary.slice().buffer);
	for (let i = 0; i < custom.vectorCount; i++) {
		const lo = 2050 + (cp[i * 3] * 32) / 7,
			hi = 2050 + ((cp[i * 3] + 1) * 32) / 7,
			y = cp[i * 3 + 1];
		let sum = [0, 0, 0],
			weight = 0;
		for (let x = 2050; x < 2082; x++) {
			const w = Math.max(0, Math.min(hi, x + 1) - Math.max(lo, x)),
				raw = [1, (x - 2050) / 10, y - 3.5],
				norm = Math.hypot(...raw);
			weight += w;
			for (let c = 0; c < 3; c++) sum[c] += (w * raw[c]) / norm;
		}
		for (let c = 0; c < 3; c++)
			assert(
				Math.abs(cv[i * 3 + c] * custom.normScale - sum[c] / weight) < 3e-6,
				'Custom ROI average mismatch'
			);
	}
	await page.getByLabel('Y first cell', { exact: true }).fill('1');
	await page.getByLabel('Y first cell', { exact: true }).blur();
	await page.getByLabel('Y end cell', { exact: true }).fill('7');
	await page.getByLabel('Y end cell', { exact: true }).blur();
	await page.getByLabel('Z first cell', { exact: true }).fill('1');
	await page.getByLabel('Z first cell', { exact: true }).blur();
	await page.getByLabel('Z end cell', { exact: true }).fill('3');
	await page.getByLabel('Z end cell', { exact: true }).blur();
	const xyz = await frameWhere(
		(f) =>
			f.regionActive &&
			f.region.start[1] === 1 &&
			f.region.end[1] === 7 &&
			f.region.start[2] === 1 &&
			f.region.end[2] === 3 &&
			f.appliedYChosenSize === 6 &&
			f.appliedZChosenSize === 2
	);
	await page.getByLabel('Quantity', { exact: true }).selectOption('alpha');
	assert(
		await page.getByRole('status').filter({ hasText: 'Loading alpha' }).isVisible(),
		'Quantity progress missing'
	);
	const alpha = await frameWhere(
		(f) => f.regionActive && f.quantity === 'alpha' && f.nComp === 1 && f.vectorCount === 84
	);
	await page.getByRole('status').filter({ hasText: 'Loading alpha' }).waitFor({ state: 'hidden' });
	assert(
		(await page.getByLabel('Color by', { exact: true }).inputValue()) === 'value',
		'alpha must activate field shader'
	);
	const av = new Float32Array(alpha.vectorValuesBinary.slice().buffer);
	for (let i = 0; i < alpha.vectorCount; i++)
		assert(Math.abs(av[i * 3] * alpha.normScale - 0.02) < 1e-7, 'alpha GPU values');
	await page
		.locator('.preview-wrapper')
		.screenshot({ path: '/tmp/3-region-alpha.png', style: '.topbar{visibility:hidden!important}' });
	await page.getByRole('button', { name: '2D section', exact: true }).click();
	await frameWhere((f) => f.type === '2D' && !f.regionActive);
	await page.getByRole('button', { name: '3D volume', exact: true }).click();
	await frameWhere((f) => f.sequence > custom.sequence && f.type === '3D' && f.regionActive);
	await page.setViewportSize({ width: 390, height: 800 });
	await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
	const canvas = await page.locator('#container').boundingBox();
	assert(canvas.height > 150, 'Mobile ROI leaves too little canvas: ' + canvas.height);
	const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
	assert(!overflow, 'Mobile horizontal overflow');
	await page.locator('.preview-wrapper').screenshot({ path: '/tmp/3-region-mobile.png' });
	await page.getByRole('button', { name: 'Exit fullscreen', exact: true }).click();
	await page.setViewportSize({ width: 1640, height: 1200 });
	// Disclosure is recreated when switching 2D -> 3D; expand when needed.
	const summary = page.locator('summary').filter({ hasText: '3D render window' });
	if (!(await summary.evaluate((e) => e.parentElement.open))) await summary.click();
	await page.getByRole('button', { name: 'Full domain', exact: true }).click();
	const restored = await frameWhere(
		(f) =>
			f.sequence > custom.sequence &&
			!f.regionActive &&
			f.appliedXChosenSize === 100 &&
			f.appliedYChosenSize === 8 &&
			f.appliedZChosenSize === 4
	);
	assert(restored.region.enabled === false, 'Region reset');
 assert(await page.getByLabel('Preview point budget',{exact:true}).isVisible(), 'Main preview budget missing');
 assert(await page.getByLabel('Maximum transferred points',{exact:true}).count()===0,'Volume transfer control should be absent');
 await page.getByLabel('Quantity',{exact:true}).selectOption('m');
 await frameWhere(f=>f.quantity==='m'&&!f.regionActive);
 await page.getByRole('button',{name:'Arrows',exact:true}).click();
 const advanced=page.locator('summary').filter({hasText:'Advanced: browser transfer'});
 await advanced.waitFor();
 assert(!(await page.getByLabel('Maximum transferred points',{exact:true}).isVisible()),'Transfer limit must be advanced');
 await advanced.click();
 assert(await page.getByLabel('Maximum transferred points',{exact:true}).isVisible(),'Advanced transfer control unavailable');
	assert(errors.length === 0, errors.join('\n'));
	await writeFile(
		new URL('native-browser.json', import.meta.url),
		JSON.stringify(
			{
				fullGrid: [full.appliedXChosenSize, 8, 4],
				nativeWindow: local.region,
				nativeGrid: [32, 8, 4],
				nativeOccupied: local.vectorCount,
				panWindow: pan.region,
				customGrid: [7, 8, 3],
				xyzWindow: xyz.region,
				alpha: {
					quantity: alpha.quantity,
					scale: alpha.normScale,
					points: alpha.vectorCount,
					colorBy: await page.getByLabel('Color by', { exact: true }).inputValue()
				},
				restoredGrid: [100, 8, 4],
				mobileCanvas: canvas,
				errors
			},
			null,
			2
		)
	);
	console.log(
		'PASS: native CUDA cells, cavity mask, UI bounds/pan, fractional crop averages, 2D restore, mobile fullscreen, full-domain restore.'
	);
} finally {
	await browser.close();
}
