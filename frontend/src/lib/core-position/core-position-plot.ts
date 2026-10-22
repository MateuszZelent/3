import { get } from 'svelte/store';
import { analyzeCore, emptyMetrics, type CoreMetrics } from './analysis';
export type { CoreMetrics } from './analysis';
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

export function getMetrics(): CoreMetrics {
	return lastMetrics;
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

// ── Analytics ───────────────────────────────────────────────

// ── Chart update ────────────────────────────────────────────

function updateCorePosition() {
	if (!chartInstance) return;

	const state = get(tablePlotState);
	const rows = state.corePos ?? [];
	const analysis = analyzeCore(rows, orbitWindowNs);
	lastMetrics = analysis.metrics;
	if (!analysis.valid.length) {
		chartInstance.setOption({ series: [] }, { replaceMerge: ['series'] });
		return;
	}
	const xArr = analysis.valid.map((r) => r[1]);
	const yArr = analysis.valid.map((r) => r[2]);
	const recentSet = new Set(analysis.recent);
	const oldData = analysis.valid.filter((r) => !recentSet.has(r)).map((r) => [r[1], r[2]]);
	const recentData = analysis.recent.map((r) => [r[1], r[2]]);
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
	for (let i = 0; i < xArr.length; i++) {
		xmin = Math.min(xmin, xArr[i]);
		xmax = Math.max(xmax, xArr[i]);
		ymin = Math.min(ymin, yArr[i]);
		ymax = Math.max(ymax, yArr[i]);
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
			formatter: (p: any) => {
				if (!p.data || p.data.length < 2) return '';
				return `x: ${(p.data[0] * 1e9).toFixed(3)} nm<br/>y: ${(p.data[1] * 1e9).toFixed(3)} nm`;
			}
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
