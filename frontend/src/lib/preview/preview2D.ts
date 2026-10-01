import { getColorScale } from './fieldColorScale';
import { previewState } from '$api/incoming/preview';
import { get, writable } from 'svelte/store';
import { preview2DWindow, clampWindow, type WindowRange } from './preview2DWindow';
import { meshState } from '$api/incoming/mesh';
import { disposePreview3D } from './preview3D';
import {
	previewSampleCoordinateNm,
	previewPlaneAxes,
	fitPreviewPlane
} from './preview2DCoordinates';
import {
	ECHARTS_THEME_NAME,
	THEME,
	ensureAmumaxEChartsTheme,
	initECharts,
	type ECharts
} from '$lib/theme/echarts-theme';

let chartInstance: ECharts | undefined;
let resizeObserver: ResizeObserver | null = null;

const AUTOSCALE_KEY = 'mumax:preview2d-autoscale';
function loadAutoscale() {
	try {
		return typeof localStorage === 'undefined' || localStorage.getItem(AUTOSCALE_KEY) !== 'false';
	} catch {
		return true;
	}
}
export const preview2DAutoscale = writable(loadAutoscale());
let windowGeometry = '';
export function setPreview2DAutoscale(enabled: boolean) {
	preview2DAutoscale.set(enabled);
	try {
		localStorage.setItem(AUTOSCALE_KEY, String(enabled));
	} catch {
		/* Storage can be unavailable. */
	}
	if (chartInstance && !chartInstance.isDisposed()) chartInstance.setOption({ grid: planeGrid() });
}
export function setPreview2DWindow(axis: 'u' | 'v', range: WindowRange) {
	const metrics = getAxisMetrics();
	const normalized = clampWindow(
		range,
		1 / (axis === 'u' ? metrics.xSampleCount : metrics.ySampleCount)
	);
	preview2DWindow.update((current) => ({ ...current, [axis]: normalized }));
	if (chartInstance && !chartInstance.isDisposed())
		chartInstance.dispatchAction({
			type: 'dataZoom',
			dataZoomId: `window-${axis}`,
			start: normalized[0] * 100,
			end: normalized[1] * 100
		});
}
export function resetPreview2DWindow() {
	setPreview2DWindow('u', [0, 1]);
	setPreview2DWindow('v', [0, 1]);
}
function syncChartWindow() {
	if (!chartInstance || chartInstance.isDisposed()) return;
	const zoom = (chartInstance.getOption().dataZoom || []) as Array<{
		id: string;
		start: number;
		end: number;
	}>;
	const current = get(preview2DWindow);
	const next = { ...current };
	for (const axis of ['u', 'v'] as const) {
		const item = zoom.find((item) => item.id === `window-${axis}`);
		if (item) next[axis] = [item.start / 100, item.end / 100];
	}
	preview2DWindow.set(next);
	chartInstance.setOption({ grid: planeGrid() });
}
function syncWindowGeometry() {
	const mesh = get(meshState),
		state = get(previewState);
	const key = `${state.plane || 'xy'}/${mesh.Nx}/${mesh.Ny}/${mesh.Nz}/${mesh.dx}/${mesh.dy}/${mesh.dz}`;
	if (key === windowGeometry) return;
	windowGeometry = key;
	preview2DWindow.set({ u: [0, 1], v: [0, 1] });
	if (chartInstance && !chartInstance.isDisposed()) resetPreview2DWindow();
}

type AxisMetrics = {
	uName: string;
	vName: string;
	xExtentNm: number;
	yExtentNm: number;
	xSampleCount: number;
	ySampleCount: number;
};

const renderWaiters = new Set<() => void>();
function waitForChart() {
	return new Promise<void>((resolve) => {
		if (!chartInstance || chartInstance.isDisposed()) {
			resolve();
			return;
		}
		const chart = chartInstance;
		const finished = () => {
			chart.off('finished', finished);
			renderWaiters.delete(finished);
			resolve();
		};
		renderWaiters.add(finished);
		chart.on('finished', finished);
	});
}

export function preview2D(): Promise<void> {
	const state = get(previewState);
	syncWindowGeometry();
	if (!state.scalarField || state.scalarField.length === 0) {
		disposePreview2D();
		disposePreview3D();
		return Promise.resolve();
	}

	// Dispose 3D renderer if it was active
	disposePreview3D();

	const container = document.getElementById('container');
	if (!container) {
		return Promise.resolve();
	}

	// Create chart instance only when truly needed (first time or after explicit dispose)
	if (chartInstance === undefined || chartInstance.isDisposed()) {
		return init();
	}

	// Keep updates incremental to avoid visible canvas resets/flicker.
	const completion = waitForChart();
	updateData();
	return completion;
}

function formatMagnitude(value: number) {
	if (!Number.isFinite(value)) {
		return 'NaN';
	}

	if (value === 0) {
		return '0';
	}

	const abs = Math.abs(value);
	if (abs >= 1000 || abs < 1e-2) {
		return value.toExponential(2);
	}
	if (abs >= 10) {
		return value.toFixed(1);
	}
	if (abs >= 1) {
		return value.toFixed(2);
	}
	return value.toPrecision(2);
}

function formatDistanceNm(distanceNm: number) {
	if (!Number.isFinite(distanceNm)) {
		return 'NaN';
	}

	const abs = Math.abs(distanceNm);
	if (abs >= 1000) {
		return distanceNm.toFixed(0);
	}

	if (abs >= 100) {
		return distanceNm.toFixed(0);
	}

	if (abs >= 10) {
		return distanceNm.toFixed(1);
	}

	return distanceNm.toFixed(2);
}

function axisPointerLabelFormatter(axis: 'x' | 'y') {
	return function (params: { value?: number }) {
		if (params.value === undefined) {
			return 'NaN';
		}
		const metrics = getAxisMetrics();
		return `${axis === 'x' ? metrics.uName : metrics.vName}: ${formatAxisCoordinate(axis, Number(params.value))} nm`;
	};
}

function buildVisualMap(quantity: string, unit: string, min: number, max: number) {
	const scale = getColorScale(min, max);
	const unitSuffix = unit ? ` ${unit}` : '';

	return {
		type: 'continuous' as const,
		min: scale.min,
		max: scale.max,
		calculable: false,
		realtime: false,
		precision: 3,
		orient: 'horizontal' as const,
		left: 'center' as const,
		bottom: 8,
		itemWidth: 10,
		itemHeight: Math.max(60, Math.min(180, (chartInstance?.getWidth() || 360) - 180)),
		align: 'right' as const,
		padding: [8, 10, 8, 10],
		textGap: 10,
		backgroundColor: 'rgba(15, 23, 42, 0.76)',
		borderColor: THEME.border,
		borderWidth: 1,
		text: [`${formatMagnitude(scale.max)}${unitSuffix}`, `${formatMagnitude(scale.min)}`],
		textStyle: {
			color: THEME.text2,
			fontSize: 11,
			fontWeight: 600
		},
		formatter: (value: number) => `${formatMagnitude(value)}${unitSuffix}`,
		inRange: {
			color: scale.palette
		},
		outOfRange: {
			color: ['rgba(107, 122, 154, 0.18)']
		},
		seriesIndex: 0,
		showLabel: true
	};
}

function getAxisMetrics(): AxisMetrics {
	const ps = get(previewState);
	const mesh = get(meshState);
	const axes = previewPlaneAxes(ps.plane);
	const counts = { x: mesh.Nx, y: mesh.Ny, z: mesh.Nz };
	const cells = { x: mesh.dx, y: mesh.dy, z: mesh.dz };
	const xChosenSize = Math.max(ps.appliedPlaneUSize || ps.appliedXChosenSize || ps.xChosenSize, 1);
	const yChosenSize = Math.max(ps.appliedPlaneVSize || ps.appliedYChosenSize || ps.yChosenSize, 1);
	const xExtentNm = cells[axes.u] * 1e9 * counts[axes.u];
	const yExtentNm = cells[axes.v] * 1e9 * counts[axes.v];

	return {
		uName: axes.u,
		vName: axes.v,
		xExtentNm,
		yExtentNm,
		xSampleCount: xChosenSize,
		ySampleCount: yChosenSize
	};
}

function axisCoordinateNm(axis: 'x' | 'y', sampleIndex: number) {
	const metrics = getAxisMetrics();
	return axis === 'x'
		? previewSampleCoordinateNm(sampleIndex, metrics.xSampleCount, metrics.xExtentNm)
		: previewSampleCoordinateNm(sampleIndex, metrics.ySampleCount, metrics.yExtentNm);
}

function formatAxisCoordinate(axis: 'x' | 'y', sampleIndex: number) {
	return formatDistanceNm(axisCoordinateNm(axis, sampleIndex));
}

function tooltipFormatter(params: any) {
	const ps = get(previewState);
	if (params.value === undefined) {
		return 'NaN';
	}
	const value = Number(params.value[2]);
	const unitSuffix = ps.unit ? ` ${ps.unit}` : '';
	return [
		`<strong>${ps.quantity}</strong>`,
		`${getAxisMetrics().uName}: ${formatAxisCoordinate('x', Number(params.value[0]))} nm`,
		`${getAxisMetrics().vName}: ${formatAxisCoordinate('y', Number(params.value[1]))} nm`,
		sectionDescription(),
		`value: ${formatMagnitude(value)}${unitSuffix}`
	].join('<br/>');
}

/** Incremental update — only series data + axis bounds + visualMap range. */
function updateData() {
	if (chartInstance === undefined || chartInstance.isDisposed()) {
		init();
		return;
	}
	const ps = get(previewState);
	const { xSampleCount, ySampleCount, uName, vName } = getAxisMetrics();
	const xCategories = Array.from({ length: xSampleCount }, (_, index) => index);
	const yCategories = Array.from({ length: ySampleCount }, (_, index) => index);
	const visualMap = buildVisualMap(ps.quantity, ps.unit, ps.min, ps.max) as any;
	chartInstance.setOption(
		{
			animation: false,
			animationDurationUpdate: 0,
			grid: planeGrid(),
			toolbox: {
				feature: {
					saveAsImage: {
						name: `preview-${ps.quantity}-${ps.plane || 'xy'}-${ps.allLayers ? 'mean' : (ps.sliceIndex ?? ps.layer)}`
					}
				}
			},
			xAxis: {
				name: `${uName} (nm)`,
				data: xCategories,
				axisLabel: {
					formatter: function (value: number) {
						return formatAxisCoordinate('x', Number(value));
					},
					hideOverlap: true
				}
			},
			yAxis: {
				name: `${vName} (nm)`,
				data: yCategories,
				axisLabel: {
					formatter: function (value: number) {
						return formatAxisCoordinate('y', Number(value));
					},
					hideOverlap: true
				}
			},
			series: [
				{
					name: ps.quantity,
					animation: false,
					progressive: ps.scalarField.length > 16384 ? 4096 : 0,
					progressiveThreshold: 16384,
					data: ps.scalarField
				}
			],
			visualMap: [visualMap]
		},
		{ lazyUpdate: true }
	);
}

function init(): Promise<void> {
	const chartDom = document.getElementById('container');
	if (!chartDom) {
		return Promise.resolve();
	}
	// Reuse existing instance if possible — avoids canvas teardown/flicker.
	if (!chartInstance || chartInstance.isDisposed()) {
		ensureAmumaxEChartsTheme();
		chartInstance = initECharts(chartDom, ECHARTS_THEME_NAME, {
			renderer: 'canvas',
			useDirtyRect: true
		});
	}
	const completion = waitForChart();
	setFullOptions();
	chartInstance.on('datazoom', syncChartWindow);
	chartInstance.on('restore', () => {
		preview2DWindow.set({ u: [0, 1], v: [0, 1] });
		chartInstance?.setOption({ grid: planeGrid() });
	});
	resizeECharts();
	return completion;
}

/** Replace all chart options on the existing instance (no canvas destruction). */
function setFullOptions() {
	if (!chartInstance || chartInstance.isDisposed()) {
		return;
	}
	const ps = get(previewState);
	const { xSampleCount, ySampleCount, uName, vName } = getAxisMetrics();
	const xCategories = Array.from({ length: xSampleCount }, (_, index) => index);
	const yCategories = Array.from({ length: ySampleCount }, (_, index) => index);
	const visualMap = buildVisualMap(ps.quantity, ps.unit, ps.min, ps.max);

	// @ts-ignore
	chartInstance.setOption(
		{
			tooltip: {
				position: 'top',
				confine: true,
				formatter: tooltipFormatter,
				backgroundColor: THEME.tooltipBg,
				borderColor: THEME.tooltipBorder,
				borderWidth: 1,
				padding: [10, 12],
				textStyle: {
					color: THEME.tooltipText,
					fontSize: 12
				}
			},
			xAxis: {
				type: 'category',
				data: xCategories,
				name: `${uName} (nm)`,
				nameLocation: 'middle',
				nameGap: 30,
				nameTextStyle: {
					color: THEME.text2,
					fontWeight: 600
				},
				axisLine: {
					show: true,
					lineStyle: {
						color: THEME.border
					}
				},
				axisPointer: {
					show: true,
					label: {
						show: true,
						backgroundColor: THEME.tooltipBg,
						color: THEME.tooltipText,
						padding: [6, 8],
						borderColor: THEME.accent,
						borderWidth: 1,
						formatter: axisPointerLabelFormatter('x')
					},
					lineStyle: {
						color: THEME.accent,
						width: 1.5,
						type: 'dashed'
					}
				},
				axisTick: {
					length: 6,
					lineStyle: {
						type: 'solid',
						color: THEME.border
					}
				},
				axisLabel: {
					show: true,
					formatter: function (value: number) {
						return formatAxisCoordinate('x', Number(value));
					},
					color: THEME.text2,
					showMinLabel: true,
					showMaxLabel: true,
					hideOverlap: true
				},
				splitLine: {
					show: false
				}
			},
			yAxis: {
				type: 'category',
				data: yCategories,
				name: `${vName} (nm)`,
				nameLocation: 'middle',
				nameGap: 54,
				nameTextStyle: {
					color: THEME.text2,
					fontWeight: 600
				},
				axisLine: {
					show: true,
					lineStyle: {
						color: THEME.border
					}
				},
				axisPointer: {
					show: true,
					label: {
						show: true,
						backgroundColor: THEME.tooltipBg,
						color: THEME.tooltipText,
						padding: [6, 8],
						borderColor: THEME.accent,
						borderWidth: 1,
						formatter: axisPointerLabelFormatter('y')
					},
					lineStyle: {
						color: THEME.accent,
						width: 1.5,
						type: 'dashed'
					}
				},
				axisTick: {
					length: 6,
					lineStyle: {
						type: 'solid',
						color: THEME.border
					}
				},
				axisLabel: {
					show: true,
					formatter: function (value: number) {
						return formatAxisCoordinate('y', Number(value));
					},
					color: THEME.text2,
					showMinLabel: true,
					showMaxLabel: true,
					hideOverlap: true
				},
				splitLine: {
					show: false
				}
			},
			visualMap: [visualMap],
			dataZoom: ['u', 'v'].map((axis, index) => ({
				id: `window-${axis}`,
				type: 'inside',
				...(index === 0 ? { xAxisIndex: 0 } : { yAxisIndex: 0 }),
				filterMode: 'filter',
				start: get(preview2DWindow)[axis as 'u' | 'v'][0] * 100,
				end: get(preview2DWindow)[axis as 'u' | 'v'][1] * 100,
				zoomOnMouseWheel: 'ctrl',
				moveOnMouseWheel: false,
				moveOnMouseMove: true,
				preventDefaultMouseMove: true
			})),
			series: [
				{
					name: ps.quantity,
					type: 'heatmap',
					selectedMode: false,
					emphasis: { disabled: true },
					// Large planes render in bounded chunks; stream ACK waits for finished.
					// Small planes retain atomic updates without progressive repainting.
					progressive: ps.scalarField.length > 16384 ? 4096 : 0,
					progressiveThreshold: 16384,
					animation: false,
					data: ps.scalarField
				}
			],
			grid: planeGrid(),
			toolbox: {
				show: true,
				top: 10,
				right: 10,
				itemSize: 20,
				itemGap: 12,
				iconStyle: {
					borderColor: THEME.toolboxIcon,
					borderWidth: 1.15
				},
				emphasis: {
					iconStyle: {
						borderColor: THEME.text1
					}
				},
				feature: {
					dataZoom: {
						xAxisIndex: 0,
						yAxisIndex: 0,
						brushStyle: {
							color: THEME.brushBg,
							borderColor: THEME.brushBorder,
							borderWidth: 2
						}
					},
					dataView: { show: false },
					restore: {
						show: true
					},
					saveAsImage: {
						type: 'png',
						name: `preview-${ps.quantity}-${ps.plane || 'xy'}-${ps.allLayers ? 'mean' : (ps.sliceIndex ?? ps.layer)}`
					}
				}
			},
			animation: false,
			animationDurationUpdate: 0
		},
		{ notMerge: true, lazyUpdate: true }
	);
}

export function disposePreview2D() {
	for (const finish of [...renderWaiters]) finish();
	if (chartInstance && !chartInstance.isDisposed()) {
		chartInstance.dispose();
	}
	chartInstance = undefined;

	if (resizeObserver) {
		resizeObserver.disconnect();
		resizeObserver = null;
	}
}

export function resizeECharts() {
	const container = document.getElementById('container');
	if (!container) {
		return;
	}

	if (!resizeObserver) {
		resizeObserver = new ResizeObserver(() => {
			if (chartInstance === undefined || chartInstance.isDisposed()) {
				return;
			}
			chartInstance.resize();
			chartInstance.setOption({
				grid: planeGrid(),
				visualMap: buildVisualMap(
					get(previewState).quantity,
					get(previewState).unit,
					get(previewState).min,
					get(previewState).max
				)
			});
		});
	}

	resizeObserver.disconnect();
	resizeObserver.observe(container);

	if (chartInstance !== undefined && !chartInstance.isDisposed()) {
		chartInstance.resize();
		chartInstance.setOption({
			grid: planeGrid(),
			visualMap: buildVisualMap(
				get(previewState).quantity,
				get(previewState).unit,
				get(previewState).min,
				get(previewState).max
			)
		});
	}
}

function planeGrid() {
	const { xExtentNm, yExtentNm } = getAxisMetrics();
	const width = chartInstance?.getWidth() || 400;
	const height = chartInstance?.getHeight() || 400;
	const availableWidth = Math.max(width - 94, 1),
		availableHeight = Math.max(height - 152, 1);
	const window = get(preview2DWindow);
	const fitted = get(preview2DAutoscale)
		? { width: availableWidth, height: availableHeight, left: 0, top: 0 }
		: fitPreviewPlane(
				availableWidth,
				availableHeight,
				xExtentNm * (window.u[1] - window.u[0]),
				yExtentNm * (window.v[1] - window.v[0])
			);
	return {
		containLabel: false,
		left: 62 + fitted.left,
		top: 54 + fitted.top,
		width: fitted.width,
		height: fitted.height
	};
}

function sectionDescription() {
	const ps = get(previewState);
	const { normal } = previewPlaneAxes(ps.plane);
	if (ps.allLayers) return `Mean along ${normal}`;
	const mesh = get(meshState);
	const cells = { x: mesh.dx, y: mesh.dy, z: mesh.dz };
	const counts = { x: mesh.Nx, y: mesh.Ny, z: mesh.Nz };
	const position = previewSampleCoordinateNm(
		ps.sliceIndex ?? ps.layer,
		counts[normal],
		cells[normal] * counts[normal] * 1e9
	);
	return `${normal}: ${formatDistanceNm(position)} nm · layer ${ps.sliceIndex ?? ps.layer}`;
}
