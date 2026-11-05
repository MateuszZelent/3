import { get } from 'svelte/store';
import {
	analyzeCore,
	emptyMetrics,
	getCorePlotCounts,
	sampleCoreCoordinates,
	type CoreMetrics,
	type CorePlotCounts
} from './analysis';
export type { CoreMetrics, CorePlotCounts } from './analysis';
import { tablePlotState } from '$api/incoming/table-plot';
import {
	ECHARTS_THEME_NAME,
	THEME,
	ensureAmumaxEChartsTheme,
	initECharts,
	type ECharts
} from '$lib/theme/echarts-theme';

let chartInstance: ECharts | undefined;
let resizeObserver: ResizeObserver | null = null;
let equalAspect = true;
let orbitWindowNs = 2; // configurable time window in nanoseconds

let lastMetrics: CoreMetrics = { ...emptyMetrics };
let lastPlotCounts: CorePlotCounts = { sourcePoints: 0, renderedPoints: 0, sampled: false };
const MAX_POINTS_PER_CORE_SERIES = 5_000;

export function getMetrics(): CoreMetrics {
	return lastMetrics;
}

export function getCorePositionPlotCounts(): CorePlotCounts {
	return { ...lastPlotCounts };
}

export function setOrbitWindow(ns: number) {
	orbitWindowNs = Math.max(0.1, ns);
	if (chartInstance) updateCorePosition();
}

export function getOrbitWindow(): number {
	return orbitWindowNs;
}

export function setEqualAspect(val: boolean) {
	equalAspect = val;
	if (chartInstance) updateCorePosition();
}

export function getEqualAspect(): boolean {
	return equalAspect;
}

export function plotCorePosition() {
	const element = document.getElementById('core-position-plot');
	if (!element) {
		disposeCorePosition();
		return;
	}
	if (chartInstance && (chartInstance.isDisposed() || chartInstance.getDom() !== element)) {
		if (!chartInstance.isDisposed()) chartInstance.dispose();
		chartInstance = undefined;
		resizeObserver?.disconnect();
	}
	if (chartInstance === undefined) {
		initCorePosition();
	}
	updateCorePosition();
}

export function disposeCorePosition() {
	resizeObserver?.disconnect();
	resizeObserver = null;
	if (chartInstance && !chartInstance.isDisposed()) chartInstance.dispose();
	chartInstance = undefined;
	lastMetrics = { ...emptyMetrics };
	lastPlotCounts = { sourcePoints: 0, renderedPoints: 0, sampled: false };
}

// ── Analytics ───────────────────────────────────────────────

// ── Chart update ────────────────────────────────────────────

function updateCorePosition() {
	if (!chartInstance) return;

	const state = get(tablePlotState);
	const rows = state.corePos ?? [];
	const analysis = analyzeCore(rows, orbitWindowNs);
	lastMetrics = analysis.metrics;
	lastPlotCounts = getCorePlotCounts(
		analysis.valid.length,
		analysis.recent.length,
		MAX_POINTS_PER_CORE_SERIES
	);
	if (!analysis.valid.length) {
		chartInstance.setOption({ series: [] }, { replaceMerge: ['series'] });
		return;
	}
	const oldPointCount = analysis.valid.length - analysis.recent.length;
	const oldData = sampleCoreCoordinates(
		analysis.valid,
		0,
		oldPointCount,
		MAX_POINTS_PER_CORE_SERIES
	);
	const recentData = sampleCoreCoordinates(
		analysis.recent,
		0,
		analysis.recent.length,
		MAX_POINTS_PER_CORE_SERIES
	);
	const cx = lastMetrics.centerXNm * 1e-9;
	const cy = lastMetrics.centerYNm * 1e-9;
	const avgRadius = lastMetrics.radiusNm * 1e-9;

	// Reference circle
	const circleData: number[][] = [];
	for (let i = 0; lastMetrics.hasData && i <= 128; i++) {
		const th = (2 * Math.PI * i) / 128;
		circleData.push([cx + avgRadius * Math.cos(th), cy + avgRadius * Math.sin(th)]);
	}

	// Center crosshair
	const crossSize = avgRadius * 0.3 || 1e-10;
	const crossH = lastMetrics.hasData
		? [
				[cx - crossSize, cy],
				[cx + crossSize, cy]
			]
		: [];
	const crossV = lastMetrics.hasData
		? [
				[cx, cy - crossSize],
				[cx, cy + crossSize]
			]
		: [];

	// Axis ranges
	let xmin = Infinity,
		xmax = -Infinity,
		ymin = Infinity,
		ymax = -Infinity;
	for (const row of analysis.valid) {
		xmin = Math.min(xmin, row[1]);
		xmax = Math.max(xmax, row[1]);
		ymin = Math.min(ymin, row[2]);
		ymax = Math.max(ymax, row[2]);
	}
	const pad = Math.max(xmax - xmin, ymax - ymin) * 0.12 || avgRadius || 1e-9;
	xmin -= pad;
	xmax += pad;
	ymin -= pad;
	ymax += pad;

	if (equalAspect) {
		const ratio =
			Math.max(1, chartInstance.getWidth() - 92) / Math.max(1, chartInstance.getHeight() - 68);
		const range = Math.max((xmax - xmin) / ratio, ymax - ymin);
		const mx = (xmin + xmax) / 2,
			my = (ymin + ymax) / 2;
		xmin = mx - (range * ratio) / 2;
		xmax = mx + (range * ratio) / 2;
		ymin = my - range / 2;
		ymax = my + range / 2;
	}

	chartInstance.setOption({
		xAxis: { min: xmin, max: xmax },
		yAxis: { min: ymin, max: ymax },
		series: [
			{
				name: 'History',
				type: 'scatter',
				data: oldData,
				symbolSize: 1.5,
				itemStyle: { color: 'rgba(96, 165, 250, 0.15)' },
				z: 1
			},
			{
				name: `Last ${orbitWindowNs} ns`,
				type: 'scatter',
				data: recentData,
				symbolSize: 2.5,
				itemStyle: { color: THEME.accent },
				z: 2
			},
			{
				name: 'Avg orbit',
				type: 'line',
				data: circleData,
				showSymbol: false,
				lineStyle: { color: '#34d399', width: 1.5, type: 'dashed' },
				itemStyle: { color: '#34d399' },
				z: 3
			},
			{
				name: 'Center',
				type: 'line',
				data: [...crossH, [null, null], ...crossV],
				showSymbol: false,
				lineStyle: { color: '#f59e0b', width: 1.5 },
				itemStyle: { color: '#f59e0b' },
				z: 4
			}
		]
	});
}

function formatCorePointTooltip(params: unknown): string {
	const item = Array.isArray(params) ? params[0] : params;
	if (!item || typeof item !== 'object' || !('data' in item) || !Array.isArray(item.data))
		return '';
	const [x, y] = item.data as unknown[];
	if (typeof x !== 'number' || typeof y !== 'number') return '';
	return `x: ${(x * 1e9).toFixed(3)} nm<br/>y: ${(y * 1e9).toFixed(3)} nm`;
}

// ── Init ────────────────────────────────────────────────────

function initCorePosition() {
	const chartDom = document.getElementById('core-position-plot');
	if (!chartDom) {
		return;
	}

	ensureAmumaxEChartsTheme();
	chartInstance = initECharts(chartDom, ECHARTS_THEME_NAME, {
		renderer: 'canvas',
		useDirtyRect: true
	});

	chartInstance.setOption({
		animation: false,
		legend: { show: false },
		tooltip: {
			trigger: 'item',
			backgroundColor: THEME.tooltipBg,
			borderColor: THEME.tooltipBorder,
			textStyle: { color: THEME.tooltipText, fontSize: 11 },
			formatter: formatCorePointTooltip
		},
		grid: {
			containLabel: true,
			left: 12,
			right: 12,
			top: 12,
			bottom: 12
		},
		xAxis: {
			name: 'x (nm)',
			nameLocation: 'middle',
			nameGap: 22,
			nameTextStyle: { color: THEME.text3, fontSize: 10 },
			axisTick: { show: false },
			axisLine: { lineStyle: { color: THEME.border } },
			splitLine: { lineStyle: { color: 'rgba(30, 45, 74, 0.4)' } },
			axisLabel: {
				formatter: (v: string) => (parseFloat(v) * 1e9).toFixed(1),
				color: THEME.text3,
				fontSize: 10
			}
		},
		yAxis: {
			name: 'y (nm)',
			nameLocation: 'middle',
			nameGap: 40,
			nameTextStyle: { color: THEME.text3, fontSize: 10 },
			axisTick: { show: false },
			axisLine: { lineStyle: { color: THEME.border } },
			splitLine: { lineStyle: { color: 'rgba(30, 45, 74, 0.4)' } },
			axisLabel: {
				formatter: (v: string) => (parseFloat(v) * 1e9).toFixed(1),
				color: THEME.text3,
				fontSize: 10
			}
		},
		series: []
	});

	if (!resizeObserver) {
		resizeObserver = new ResizeObserver(() => {
			if (chartInstance && !chartInstance.isDisposed()) {
				chartInstance.resize();
				updateCorePosition();
			}
		});
	}
	resizeObserver.disconnect();
	resizeObserver.observe(chartDom);
	chartInstance.resize();
}
