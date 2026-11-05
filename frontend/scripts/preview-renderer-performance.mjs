#!/usr/bin/env node
import { chromium } from '@playwright/test';
import { readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import process from 'node:process';
import { createServer } from 'vite';

const port = Number(process.env.PREVIEW_BENCH_PORT || 4175);
const outputPath = resolve(
	process.env.PREVIEW_BENCH_OUTPUT || `preview-renderer-performance-${Date.now()}.json`
);
const counts = (process.env.PREVIEW_BENCH_COUNTS || '128000,1000000').split(',').map(Number);
/** @typedef {{count:number, firstFrameDrawn?:boolean, visibleAfterVoxelMount?:number, voxelMountMs:number, glyphSwitchMs:number, colorUploadBytes?:number, cameraUpdateMsDelta?:number, cameraUploadBytes?:number, uploadedBeforeCamera?:number, qualityUpgrade?:{contextAntialias?:boolean|null, canvasReplaced?:boolean, cameraPreserved?:boolean}, qualitySameAntialiasCanvasPreserved?:boolean, scalarFixtureMs?:number, chartMountMs?:number|null, chartIncrementalMs?:number|null, sameDataReplayMs?:number, sameDataFrameAcks?:Array<{sequence:number,configRevision?:number,resolved:boolean,elapsedMs:number}>, chartSkipped?:string, rasterSmoke?:{centerPixel?:number[]|null, tooltip?:string}, longTasksByOperation?:Record<string, Array<{duration:number}>>, longTasks?:Array<{duration:number}>}} BenchmarkResult */
const benchmarkPagePath = '/__preview_renderer_benchmark__.html';
const benchmarkHtml = await readFile(resolve('renderer-harness.html'), 'utf8');
let vite;
let browser;
try {
	vite = await createServer({
		configFile: resolve('vite.config.ts'),
		server: { host: '127.0.0.1', port, strictPort: true },
		plugins: [
			{
				name: 'preview-renderer-benchmark-page',
				configureServer(server) {
					server.middlewares.use(benchmarkPagePath, async (_request, response, next) => {
						try {
							const html = await server.transformIndexHtml(benchmarkPagePath, benchmarkHtml);
							response.statusCode = 200;
							response.setHeader('Content-Type', 'text/html');
							response.end(html);
						} catch (error) {
							next(error instanceof Error ? error : new Error(String(error)));
						}
					});
				}
			}
		]
	});
	await vite.listen();
	browser = await chromium.launch({
		headless: true,
		args: ['--enable-webgl', '--use-gl=swiftshader', '--no-sandbox']
	});
	const page = await browser.newPage({ viewport: { width: 1100, height: 780 } });
	/** @type {string[]} */
	const consoleErrors = [];
	page.on('pageerror', (error) => consoleErrors.push(String(error)));
	page.on('console', (message) => {
		if (message.type() === 'error') consoleErrors.push(message.text());
	});
	await page.route('http://127.0.0.1:4174/api/**', (route) => route.fulfill({ json: {} }));
	await page.routeWebSocket(/\/ws(?:\?.*)?$/, (socket) => socket.close());
	await page.goto(`http://127.0.0.1:${port}${benchmarkPagePath}`, {
		waitUntil: 'load',
		timeout: 60000
	});
	await page.waitForTimeout(3000);
	const hasHarness = await page.evaluate(() => {
		const browserWindow =
			/** @type {Window & { runPreviewRendererBench?: (counts: number[]) => Promise<unknown> }} */ (
				window
			);
		return typeof browserWindow.runPreviewRendererBench === 'function';
	});
	if (!hasHarness) {
		throw new Error(
			`Harness module did not load; url=${page.url()} body=${(await page.locator('body').innerText()).slice(0, 500)} pageErrors=${consoleErrors.join(' | ')}`
		);
	}
	const results = /** @type {BenchmarkResult[]} */ (
		await page.evaluate((requestedCounts) => {
			const browserWindow =
				/** @type {Window & { runPreviewRendererBench?: (counts: number[]) => Promise<unknown> }} */ (
					window
				);
			return browserWindow.runPreviewRendererBench?.(requestedCounts);
		}, counts)
	);
	const failures = [];
	const small = results.find((result) => result.count === 128000);
	const large = results.find((result) => result.count === 1000000);
	if (!small || !large) failures.push('Expected 128000 and 1000000 point renderer cases.');
	if (small) {
		if (!small.firstFrameDrawn)
			failures.push('3D preview promise did not resolve true after drawing the first 128k frame.');
		if (small.visibleAfterVoxelMount !== small.count)
			failures.push(
				`3D visible instance count ${small.visibleAfterVoxelMount} did not match ${small.count}.`
			);
		if (small.colorUploadBytes !== 0)
			failures.push(`Color-only update uploaded ${small.colorUploadBytes} bytes.`);
		if (small.cameraUpdateMsDelta !== 0 || small.cameraUploadBytes !== small.uploadedBeforeCamera)
			failures.push('Camera-only update rebuilt or re-uploaded instance data.');
		if (
			!small.qualityUpgrade?.contextAntialias ||
			!small.qualityUpgrade?.canvasReplaced ||
			!small.qualityUpgrade?.cameraPreserved
		)
			failures.push(
				'Low-to-high quality did not replace the context with antialias while preserving the camera.'
			);
		if (!small.qualitySameAntialiasCanvasPreserved)
			failures.push(
				'High-to-ultra quality unnecessarily recreated an antialias-equivalent context.'
			);
		if ((small.chartMountMs ?? Infinity) > 500 || (small.chartIncrementalMs ?? Infinity) > 500)
			failures.push('128k 2D raster update exceeded the 500 ms regression budget.');
		if ((small.rasterSmoke?.centerPixel?.[3] ?? 0) === 0)
			failures.push('2D raster plot center is transparent.');
		if (!small.rasterSmoke?.tooltip?.includes('value: 0.16 A/m'))
			failures.push(
				`2D exact-coordinate tooltip mismatch: ${small.rasterSmoke?.tooltip || 'missing'}`
			);
		if (
			small.sameDataFrameAcks?.length !== 2 ||
			small.sameDataFrameAcks.some((frame) => !frame.resolved || frame.configRevision !== 10)
		)
			failures.push(
				'Repeated 2D same-data frames did not settle with the unchanged config revision.'
			);
	}
	if (large && (large.voxelMountMs > 1000 || large.glyphSwitchMs > 1000))
		failures.push('1M 3D render-mode update exceeded the 1000 ms measurement budget.');
	if (large && (!large.firstFrameDrawn || large.visibleAfterVoxelMount !== large.count))
		failures.push('3D 1M frame did not draw all synthetic voxel instances.');
	if (large && (large.chartMountMs ?? Infinity) > 2000)
		failures.push('1M 2D dense raster mount exceeded the 2000 ms measurement budget.');
	if (large && (large.chartIncrementalMs ?? Infinity) > 2000)
		failures.push('1M 2D dense raster update exceeded the 2000 ms measurement budget.');
	if (large && !large.chartSkipped && (large.rasterSmoke?.centerPixel?.[3] ?? 0) === 0)
		failures.push('1M 2D raster plot sample is transparent.');
	if (large && !large.chartSkipped && !large.rasterSmoke?.tooltip?.includes('value: 0.16 A/m'))
		failures.push(
			`1M 2D exact-coordinate tooltip mismatch: ${large.rasterSmoke?.tooltip || 'missing'}`
		);
	if (
		large &&
		!large.chartSkipped &&
		(!large.rasterSmoke?.tooltip?.includes('x: 87.5 nm') ||
			!large.rasterSmoke?.tooltip?.includes('y: 103 nm'))
	)
		failures.push('1M 2D hover did not report the exact source-cell coordinates.');
	if (
		large &&
		(large.sameDataFrameAcks?.length !== 2 ||
			large.sameDataFrameAcks.some((frame) => !frame.resolved || frame.configRevision !== 10))
	)
		failures.push(
			'Repeated 1M 2D same-data frames did not settle with the unchanged config revision.'
		);
	const measuredLongTasks = results.flatMap((result) =>
		Object.values(result.longTasksByOperation || {}).flat()
	);
	const longestTask = Math.max(0, ...measuredLongTasks.map((task) => task.duration));
	if (longestTask > 250)
		failures.push(`A measured operation produced a ${longestTask} ms Long Task (budget 250 ms).`);
	if (consoleErrors.length)
		failures.push(`Browser reported ${consoleErrors.length} console/page errors.`);
	const report = {
		generatedAt: new Date().toISOString(),
		userAgent: await page.evaluate(() => navigator.userAgent),
		viewport: { width: 1100, height: 780 },
		gpuDraw:
			'WebGLRenderer.render is patched to a no-op; results measure main-thread setup/update and ECharts completion, not GPU draw time.',
		counts,
		results,
		assertions: { passed: failures.length === 0, failures, longestTaskMs: longestTask },
		consoleErrors,
		viteOutput: 'Vite benchmark page is provided by an in-memory development-only middleware.'
	};
	await writeFile(outputPath, `${JSON.stringify(report, null, 2)}\n`);
	process.stdout.write(
		`${JSON.stringify({ outputPath, assertions: report.assertions, results, consoleErrors }, null, 2)}\n`
	);
	if (failures.length) {
		process.stderr.write(
			`Renderer regression assertions failed:\n${failures.map((failure) => `- ${failure}`).join('\n')}\n`
		);
		process.exitCode = 1;
	}
} catch (error) {
	const message = error instanceof Error ? error.stack || error.message : String(error);
	process.stderr.write(`${message}\n`);
	process.exitCode = 1;
} finally {
	if (browser) await browser.close();
	if (vite) await vite.close();
}
