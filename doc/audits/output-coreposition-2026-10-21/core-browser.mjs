import { chromium } from '../../../frontend/node_modules/playwright/index.mjs';
import { decode } from '../../../frontend/node_modules/@msgpack/msgpack/dist.esm/index.mjs';
import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const url = process.env.CORE_URL;
if (!url) throw Error('Set CORE_URL to the interactive test solver');
const output = process.env.CORE_OUTPUT_DIR || fileURLToPath(new URL('../../../build/core-browser-output/', import.meta.url));
const artifacts = fileURLToPath(new URL('./', import.meta.url));
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1600, height: 1100 } });
const errors = [], reports = [];
let latest, connections = 0, clientRoute, serverRoute;
page.on('pageerror', error => errors.push(error.message));
page.on('console', message => { if (message.type() === 'error') errors.push(message.text()); });
await page.routeWebSocket(u => u.pathname === '/ws', ws => {
  connections++;
  clientRoute = ws;
  const server = ws.connectToServer();
  serverRoute = server;
  server.onMessage(message => {
    if (typeof message !== 'string') {
      const packet = decode(new Uint8Array(message));
      if (packet.tablePlot) latest = packet.tablePlot;
    }
    ws.send(message);
  });
});

async function until(condition, label) {
  const deadline = Date.now() + 20000;
  while (!(await condition())) {
    if (Date.now() > deadline) throw Error(`Timed out: ${label}`);
    await page.waitForTimeout(50);
  }
}
async function command(command) {
  const result = await page.request.post(url + '/api/console/command', { data: { command } });
  assert(result.ok(), await result.text());
}
async function openCore() {
  await page.getByRole('button', { name: 'Core Position', exact: true }).click();
  await page.locator('#core-position-plot canvas').first().waitFor();
}
async function capture(phase, valid) {
  await until(() => latest?.corePos?.length && latest.corePos.at(-1)[3] === Number(valid), phase + ' transport');
  assert.equal(latest.coreMode, 'skyrmion');
  assert(latest.corePos.every(row => row.every(Number.isFinite)), 'nonfinite wire coordinates');
  await page.waitForTimeout(200);
  if (valid) {
    await until(async () => (await page.locator('.cp__metrics').count()) === 1, phase + ' metrics');
    const values = await page.locator('.cp__metric-value').allTextContents();
    assert(values.includes('11.25') && values.includes('-7.75'), values.join(', '));
    assert(!values.some(value => value.includes('NaN') || value.includes('Infinity')));
    assert(values.includes('—'), 'stationary core must not have a claimed frequency');
  } else {
    await until(async () => (await page.locator('.cp__metrics').count()) === 0, phase + ' invalid metrics');
    assert.notEqual(latest.coreStatus, 'tracking');
  }
  const pixels = await page.locator('#core-position-plot canvas').first().evaluate(canvas => {
    const data = canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height).data;
    const colors = new Set();
    for (let i = 0; i < data.length; i += 16) colors.add(`${data[i]},${data[i+1]},${data[i+2]},${data[i+3]}`);
    return colors.size;
  });
  assert(pixels > 4, 'core plot canvas is blank');
  await page.locator('#core-position-plot').screenshot({ path: artifacts + phase + '.png' });
  reports.push({ phase, valid, points: latest.corePos.length, status: latest.coreStatus, canvasColors: pixels });
}

try {
  await page.goto(url);
  await until(() => latest?.coreMode === 'skyrmion', 'initial core state');
  await command('SkyrmionReset(); m=NeelSkyrmion(1,1).Transl(11.25e-9,-7.75e-9,0); t=0; TableSave(); Flush()');
  await openCore();
  await capture('core-valid', true);
  await command('m=Uniform(0,0,1); t=1e-9; TableSave(); Flush()');
  await capture('core-invalid', false);
  await command('SkyrmionReset(); m=BlochSkyrmion(1,1).Transl(11.25e-9,-7.75e-9,0); t=2e-9; TableSave(); Flush()');
  await capture('core-recovered', true);

  const before = connections;
  clientRoute.close({ code: 1012, reason: 'audit reconnect' });
  serverRoute.close({ code: 1012, reason: 'audit reconnect' });
  await until(() => connections > before && latest?.coreMode === 'skyrmion', 'main websocket reconnect');
  await openCore();
  await capture('core-reconnected', true);

  await command('FixDt=1e-12; TableAutoSave(1e-12)');
  const manifest = async () => JSON.parse(await readFile(output + '/table/manifest.json', 'utf8'));
  const prior = (await manifest()).committed_rows;
  const run = page.request.post(url + '/api/solver/run', { data: { runtime: '1000' }, timeout: 30000 });
  await until(async () => (await manifest()).committed_rows > prior, 'actual solver running');
  const stopped = await page.request.post(url + '/api/solver/break', { data: {} });
  assert(stopped.ok(), await stopped.text());
  assert((await run).ok(), 'run request failed after Break');
  const committed = await manifest();
  const text = (await readFile(output + '/table.txt', 'utf8')).trimEnd().split('\n');
  assert.equal(text.length - 1, committed.committed_rows, 'Break returned before table persistence');
  assert(text.slice(1).every(line => line.trim().split(/\s+/).length === committed.columns.length), 'empty or truncated text rows');
  reports.push({ phase: 'break', committedRows: committed.committed_rows, textRows: text.length - 1, flushedBeforeResponse: true });
  assert.deepEqual(errors, []);
  await writeFile(artifacts + 'browser-result.json', JSON.stringify({ reports, connections, errors }, null, 2) + '\n');
  console.log('PASS', JSON.stringify({ reports, connections }));
} catch (error) {
  console.error('UI errors:', errors);
  console.error('Buttons:', await page.locator('button').allTextContents());
  console.error('Page:', (await page.locator('body').innerText()).slice(-5000));
  await page.screenshot({ path: artifacts + 'core-browser-failure.png' });
  throw error;
} finally {
  await browser.close();
}
