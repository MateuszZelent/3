const { chromium } = await import(
	new URL('../../../frontend/node_modules/playwright/index.mjs', import.meta.url)
);
const baseUrl = process.env.PREVIEW_URL || 'http://127.0.0.1:35367';
import { writeFile } from 'node:fs/promises';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1800, height: 1400 } });
const errors = [],
	socketEvents = [];
page.on('pageerror', (e) => errors.push(e.message));
page.on('websocket', (ws) => {
	socketEvents.push({ action: 'open', url: ws.url() });
	ws.on('close', () => socketEvents.push({ action: 'close', url: ws.url() }));
});
const result = [];
const waitCount = async (n, grid = '') =>
	page.waitForFunction(
		({ n, grid }) => {
			const text = (document.querySelector('.preview-wrapper__stats')?.textContent || '').replace(/\s+/g, ' ');
			return text.includes('Arrows: ' + n.toLocaleString()) && (!grid || text.includes(grid));
		},
		{ n, grid },
		{ timeout: 90000 }
	);

const record = async (name) => {
	const text = await page.locator('.preview-wrapper__stats').innerText();
	if (name.startsWith('volume-') && !text.includes('grid 100 × 100 × 100'))
		throw Error('Incorrect volume grid: ' + text);
	if (text.includes('/ 0 server points')) throw Error('Missing server count in ' + name);
	result.push({ name, text });
	console.log(name + ': ' + text);
};
try {
	await page.goto(baseUrl + '/');
	await page.getByRole('button', { name: 'Full mesh resolution', exact: true }).waitFor();
	await page.locator('#container').scrollIntoViewIfNeeded();
	await page.getByRole('button', { name: 'LOW', exact: true }).click();
	if (await page.getByLabel('Auto-adjust resolution', { exact: true }).isChecked())
		await page.locator('label.ui-toggle').filter({ hasText: 'Auto-adjust resolution' }).click();
	await page.getByRole('button', { name: 'Full mesh resolution', exact: true }).click();
	await waitCount(1000000);
	await record('manual-full');
	await page.getByLabel('Transfer limit', { exact: true }).selectOption('131072');
	await waitCount(62500);
	await record('explicit-128k-transfer');
	await page.getByLabel('Transfer limit', { exact: true }).selectOption('1000000');
	await waitCount(1000000);
	await record('restored-full-transfer');
	await page.locator('label.ui-toggle').filter({ hasText: 'Auto-adjust resolution' }).click();
	await page.getByLabel('Auto-adjust budget', { exact: true }).selectOption('500000');
	await waitCount(499849);
	await record('auto-500k');
	await page.getByLabel('Auto-adjust budget', { exact: true }).selectOption('1000000');
	await waitCount(1000000);
	await record('auto-1m');
	await page.getByLabel('Field scale', { exact: false }).fill('4');
	await page.getByLabel('Field scale', { exact: false }).press('Tab');
	await page.waitForFunction(() =>
		document.querySelector('.preview-wrapper__stats')?.textContent?.includes('scale 4.00')
	);
	await page
		.locator('.preview-toolbar')
		.screenshot({ path: '/tmp/3-preview-limits-appearance.png' });
	const volumeResponse = await page.request.post(baseUrl + '/api/console/command', {
		data: { command: 'SetGridSize(100,100,100); m=Uniform(1,0.5,-0.25)' }
	});
	if (!volumeResponse.ok()) throw Error(await volumeResponse.text());
	await page.getByRole('button', { name: 'All layers', exact: true }).click();
	if (await page.getByLabel('Auto-adjust resolution', { exact: true }).isChecked())
		await page.locator('label.ui-toggle').filter({ hasText: 'Auto-adjust resolution' }).click();
	await page.getByRole('button', { name: 'Full mesh resolution', exact: true }).click();
	await waitCount(1000000, 'grid 100 × 100 × 100');
	await record('volume-100x100x100-full');
	await page.getByLabel('Transfer limit', { exact: true }).selectOption('131072');
	await waitCount(125000);
	await record('volume-128k-transfer');
	await page.getByLabel('Transfer limit', { exact: true }).selectOption('1000000');
	await waitCount(1000000, 'grid 100 × 100 × 100');
	await record('volume-restored-full');
	if (socketEvents.some((e) => e.action === 'close' && e.url.endsWith('/preview')))
		throw Error('Preview disconnected during slow rendering');
	const edgeResponse = await page.request.post(baseUrl + '/api/console/command', {
		data: { command: 'SetGridSize(64,64,256); m=Uniform(1,0.5,-0.25)' }
	});
	if (!edgeResponse.ok()) throw Error(await edgeResponse.text());
	await page.getByRole('button', { name: 'Full mesh resolution', exact: true }).click();
	await waitCount(984064, 'grid 62 × 62 × 256');
	await record('near-hard-limit-keeps-256-layers');
	await page.getByText('Safety limit', { exact: true }).waitFor();
	if (socketEvents.some((e) => e.action === 'close' && e.url.endsWith('/preview')))
		throw Error('Preview disconnected during slow rendering');
	await page.reload();
	await page.locator('#container').scrollIntoViewIfNeeded();
	await waitCount(984064, 'grid 62 × 62 × 256');
	await record('reconnected-near-hard-limit');
	if (errors.length) throw Error(errors.join('\n'));
	await writeFile(
		process.env.PREVIEW_REPORT || new URL('preview-limits-browser.json', import.meta.url),
		JSON.stringify({ result, errors, socketEvents }, null, 2)
	);
} catch (error) {
	console.error(
		await page
			.locator('.preview-wrapper__stats')
			.innerText()
			.catch(() => '')
	);
	console.error(
		await page
			.locator('.preview-resolution-summary')
			.innerText()
			.catch(() => '')
	);
	console.error(JSON.stringify(socketEvents));
	throw error;
} finally {
	await browser.close();
}
