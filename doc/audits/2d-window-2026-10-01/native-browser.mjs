import { chromium } from '../../../frontend/node_modules/playwright/index.mjs';
import { decode } from '../../../frontend/node_modules/@msgpack/msgpack/dist.esm/index.mjs';
import { writeFile } from 'node:fs/promises';
const url = process.env.PREVIEW_URL || 'http://127.0.0.1:35367';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1640, height: 1100 } });
const frames = [],
	errors = [];
page.on('pageerror', (error) => errors.push(error.message));
page.on('websocket', (socket) => {
	if (socket.url().endsWith('/preview'))
		socket.on('framereceived', (event) => frames.push(decode(new Uint8Array(event.payload))));
});
const assert = (condition, message) => {
	if (!condition) throw Error(message);
};
try {
	await page.goto(url);
	await page.getByRole('button', { name: '2D section', exact: true }).click();
	await page.getByLabel('Autoscale', { exact: true }).waitFor();
	await page.waitForFunction(() => document.querySelector('#container canvas'));
	await page.waitForTimeout(500);
	assert(await page.getByLabel('Autoscale', { exact: true }).isChecked(), 'Autoscale default');
	await page.getByLabel('X window end', { exact: true }).fill('0.5');
	await page.getByLabel('X window start', { exact: true }).fill('0.25');
	const selection = page.getByRole('slider', { name: 'Move X window', exact: true });
	await selection.scrollIntoViewIfNeeded();
	const before = await selection.getAttribute('aria-valuetext'),
		box = await selection.boundingBox();
	await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
	await page.mouse.down();
	await page.mouse.move(box.x + box.width / 2 + 35, box.y + box.height / 2, { steps: 8 });
	await page.mouse.up();
	const after = await selection.getAttribute('aria-valuetext');
	assert(before !== after, 'Pan moved window');
	await page.waitForTimeout(1200);
	assert(
		(await selection.getAttribute('aria-valuetext')) === after,
		'Streaming frames preserve window'
	);
	await page
		.locator('[data-panel="preview"]')
		.screenshot({
			path: '/tmp/3-window-native.png',
			style: '.topbar{visibility:hidden!important}'
		});
	await page.getByRole('button', { name: 'Full window', exact: true }).click();
	assert(
		(await page.getByLabel('X window start', { exact: true }).inputValue()) === '0',
		'Reset start'
	);
	assert(
		(await page.getByLabel('X window end', { exact: true }).inputValue()) === '1',
		'Reset end'
	);
	const frame = frames.findLast((frame) => frame.type === '2D');
	assert(frame?.scalarField.length > 0, 'Real CUDA preview frame');
	assert(errors.length === 0, errors.join('\n'));
	await writeFile(
		new URL('native-browser.json', import.meta.url),
		JSON.stringify(
			{
				mesh: [4096, 8, 2],
				before,
				after,
				frame: {
					plane: frame.plane,
					points: frame.scalarField.length,
					u: frame.appliedPlaneUSize,
					v: frame.appliedPlaneVSize
				},
				errors
			},
			null,
			2
		)
	);
} finally {
	await browser.close();
}
