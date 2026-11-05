import { getColorScale } from './fieldColorScale';
import { previewState, type Preview } from '$api/incoming/preview';
import { get, writable } from 'svelte/store';
import { preview2DWindow, clampWindow, type WindowRange } from './preview2DWindow';
import { meshState, type Mesh } from '$api/incoming/mesh';
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
import { buildScalarHeatmapRaster, type ScalarHeatmapRaster } from './scalarHeatmapRaster';
import { readPreviewStorage, writePreviewStorage } from './safePreviewStorage';
import { previewFrameMesh } from './previewFrameMesh';
import { use as registerEChartsComponents } from 'echarts/core';
import { GraphicComponent } from 'echarts/components';
import type {
	ElementEvent,
	GraphicComponentOption,
	TooltipComponentFormatterCallbackParams,
	VisualMapComponentOption
} from 'echarts';

registerEChartsComponents([GraphicComponent]);

type PreviewChartOption = Parameters<ECharts['setOption']>[0] & {
	graphic?: GraphicComponentOption | GraphicComponentOption[];
};

let chartInstance: ECharts | undefined;
let resizeObserver: ResizeObserver | null = null;
let scalarRaster: ScalarHeatmapRaster | null = null;
let rasterPointerHandler: ((event: ElementEvent) => void) | null = null;
let rasterGlobalOutHandler: (() => void) | null = null;
let rasterFrameId: number | null = null;
let rasterTooltip: HTMLDivElement | null = null;
let observedContainer: HTMLElement | null = null;
let lastChartWidth = 0;
let lastChartHeight = 0;

const AUTOSCALE_KEY = 'mumax:preview2d-autoscale';
function loadAutoscale() {
	return readPreviewStorage(AUTOSCALE_KEY) !== 'false';
}
export const preview2DAutoscale = writable(loadAutoscale());
let windowGeometry = '';
export function setPreview2DAutoscale(enabled: boolean) {
	preview2DAutoscale.set(enabled);
	writePreviewStorage(AUTOSCALE_KEY, String(enabled));
	if (chartInstance && !chartInstance.isDisposed()) scheduleRasterUpdate();
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
	scheduleRasterUpdate();
}
function syncWindowGeometry() {
	const mesh = getFrameMesh(),
		state = get(previewState);
	const key = `${state.plane || 'xy'}/${mesh.Nx}/${mesh.Ny}/${mesh.Nz}/${mesh.dx}/${mesh.dy}/${mesh.dz}`;
	if (key === windowGeometry) return;
	windowGeometry = key;
	preview2DWindow.set({ u: [0, 1], v: [0, 1] });
	if (chartInstance && !chartInstance.isDisposed()) resetPreview2DWindow();
}

function getFrameMesh(): Mesh {
	const state = get(previewState) as Preview & { mesh?: Mesh };
	return previewFrameMesh(state, get(meshState));
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

function buildCurrentRaster() {
	const state = get(previewState);
	const metrics = getAxisMetrics();
	const scale = getColorScale(state.min, state.max);
	scalarRaster = buildScalarHeatmapRaster(
		state.scalarField,
		metrics.xSampleCount,
		metrics.ySampleCount,
		scale.min,
		scale.max,
		scale.palette,
		{
			window: get(preview2DWindow),
			viewportWidth: planeGrid().width,
			viewportHeight: planeGrid().height,
			pixelRatio: Math.min(window.devicePixelRatio || 1, 2)
		}
	);
}

function rasterGraphic(): GraphicComponentOption[] {
	if (!scalarRaster || !chartInstance || chartInstance.isDisposed()) return [];
	const canvas = document.createElement('canvas');
	canvas.width = scalarRaster.width;
	canvas.height = scalarRaster.height;
	const context = canvas.getContext('2d', { alpha: true });
	if (!context) return [];
	const image = context.createImageData(scalarRaster.width, scalarRaster.height);
	image.data.set(scalarRaster.pixels);
	context.putImageData(image, 0, 0);
	const grid = planeGrid();
	return [
		{
			id: 'scalar-heatmap-raster',
			type: 'image',
			silent: true,
			z: -10,
			style: {
				image: canvas,
				x: grid.left,
				y: grid.top,
				width: grid.width,
				height: grid.height
			}
		}
	];
}

function scheduleRasterUpdate() {
	if (rasterFrameId !== null || !chartInstance || chartInstance.isDisposed()) return;
	rasterFrameId = requestAnimationFrame(() => {
		rasterFrameId = null;
		if (!chartInstance || chartInstance.isDisposed()) return;
		buildCurrentRaster();
		chartInstance.setOption({ grid: planeGrid(), graphic: rasterGraphic() });
	});
}

function hideRasterTooltip() {
	if (rasterTooltip) rasterTooltip.style.display = 'none';
}

function handleRasterPointerMove(event: ElementEvent) {
	if (!chartInstance || chartInstance.isDisposed() || !scalarRaster) return;
	const pointX = event.offsetX,
		pointY = event.offsetY;
	const grid = planeGrid();
	if (
		!Number.isFinite(pointX) ||
		!Number.isFinite(pointY) ||
		pointX < grid.left ||
		pointX > grid.left + grid.width ||
		pointY < grid.top ||
		pointY > grid.top + grid.height
	) {
		hideRasterTooltip();
		return;
	}
	const window = get(preview2DWindow);
	const metrics = getAxisMetrics();
	const u =
		window.u[0] + ((pointX - grid.left) / Math.max(grid.width, 1)) * (window.u[1] - window.u[0]);
	const v =
		window.v[1] - ((pointY - grid.top) / Math.max(grid.height, 1)) * (window.v[1] - window.v[0]);
	const x = Math.max(0, Math.min(metrics.xSampleCount - 1, Math.floor(u * metrics.xSampleCount)));
	const y = Math.max(0, Math.min(metrics.ySampleCount - 1, Math.floor(v * metrics.ySampleCount)));
	const index = scalarRaster.getSourceIndex(x, y);
	if (index < 0) {
		hideRasterTooltip();
		return;
	}
	const sample = get(previewState).scalarField[index];
	if (!sample) {
		hideRasterTooltip();
		return;
	}
	if (!rasterTooltip) {
		rasterTooltip = document.createElement('div');
		rasterTooltip.setAttribute('role', 'tooltip');
		rasterTooltip.style.cssText = [
			'position:fixed',
			'z-index:10000',
			'display:none',
			'pointer-events:none',
			'padding:9px 11px',
			'border:1px solid ' + THEME.tooltipBorder,
			'border-radius:6px',
			'background:' + THEME.tooltipBg,
			'color:' + THEME.tooltipText,
			'font:12px/1.45 system-ui,sans-serif',
			'white-space:pre-line',
			'box-shadow:0 5px 18px #0005'
		].join(';');
		document.body.appendChild(rasterTooltip);
	}
	const state = get(previewState);
	rasterTooltip.textContent = [
		state.quantity,
		`${getAxisMetrics().uName}: ${formatAxisCoordinate('x', x)} nm`,
		`${getAxisMetrics().vName}: ${formatAxisCoordinate('y', y)} nm`,
		sectionDescription(),
		`value: ${formatMagnitude(Number(sample[2]))}${state.unit ? ` ${state.unit}` : ''}`
	].join('\n');
	const container = document.getElementById('container');
	const rect = container?.getBoundingClientRect();
	rasterTooltip.style.left = `${(rect?.left ?? 0) + pointX + 14}px`;
	rasterTooltip.style.top = `${(rect?.top ?? 0) + pointY + 14}px`;
	rasterTooltip.style.display = 'block';
	chartInstance.dispatchAction({ type: 'updateAxisPointer', x: pointX, y: pointY });
}

function attachRasterPointerHandlers() {
	if (!chartInstance || chartInstance.isDisposed()) return;
	const zr = chartInstance.getZr();
	rasterPointerHandler = handleRasterPointerMove;
	rasterGlobalOutHandler = hideRasterTooltip;
	zr.on('mousemove', rasterPointerHandler);
	zr.on('globalout', rasterGlobalOutHandler);
}

function detachRasterPointerHandlers() {
	if (chartInstance && !chartInstance.isDisposed()) {
		const zr = chartInstance.getZr();
		if (rasterPointerHandler) zr.off('mousemove', rasterPointerHandler);
		if (rasterGlobalOutHandler) zr.off('globalout', rasterGlobalOutHandler);
	}
	rasterPointerHandler = null;
	rasterGlobalOutHandler = null;
	rasterTooltip?.remove();
	rasterTooltip = null;
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

function buildVisualMap(
	quantity: string,
	unit: string,
	min: number,
	max: number
): VisualMapComponentOption {
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
		backgroundColor: THEME.surface1,
		borderColor: THEME.border,
		borderWidth: 1,
		text: [`${formatMagnitude(scale.max)}${unitSuffix}`, `${formatMagnitude(scale.min)}`],
		textStyle: {
			color: THEME.text2,
			fontSize: 11,
			fontWeight: 600
		},
		formatter: (value: string | number | Date | null | undefined) =>
			`${formatMagnitude(Number(value))}${unitSuffix}`,
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
	const mesh = getFrameMesh();
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

function tooltipFormatter(params: TooltipComponentFormatterCallbackParams) {
	const item = Array.isArray(params) ? params[0] : params;
	const coordinates = item && Array.isArray(item.value) ? item.value : [];
	if (coordinates.length < 3) return 'NaN';
	const ps = get(previewState);
	const value = Number(coordinates[2]);
	const unitSuffix = ps.unit ? ` ${ps.unit}` : '';
	return [
		`<strong>${ps.quantity}</strong>`,
		`${getAxisMetrics().uName}: ${formatAxisCoordinate('x', Number(coordinates[0]))} nm`,
		`${getAxisMetrics().vName}: ${formatAxisCoordinate('y', Number(coordinates[1]))} nm`,
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
	const visualMap = buildVisualMap(ps.quantity, ps.unit, ps.min, ps.max);
	buildCurrentRaster();
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
					type: 'heatmap',
					animation: false,
					data: []
				}
			],
			visualMap: [visualMap],
			graphic: rasterGraphic()
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
	attachRasterPointerHandlers();
	const completion = waitForChart();
	setFullOptions();
	chartInstance.on('datazoom', syncChartWindow);
	chartInstance.on('restore', () => {
		preview2DWindow.set({ u: [0, 1], v: [0, 1] });
		scheduleRasterUpdate();
	});
	observeChartContainer(chartDom);
	lastChartWidth = chartInstance.getWidth();
	lastChartHeight = chartInstance.getHeight();
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
	buildCurrentRaster();

	const options: PreviewChartOption = {
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
				animation: false,
				data: []
			}
		],
		graphic: rasterGraphic(),
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
	};
	chartInstance.setOption(options, { notMerge: true, lazyUpdate: true });
}

export function disposePreview2D() {
	for (const finish of [...renderWaiters]) finish();
	detachRasterPointerHandlers();
	if (chartInstance && !chartInstance.isDisposed()) {
		chartInstance.dispose();
	}
	chartInstance = undefined;
	scalarRaster = null;
	if (rasterFrameId !== null) {
		cancelAnimationFrame(rasterFrameId);
		rasterFrameId = null;
	}

	if (resizeObserver) {
		resizeObserver.disconnect();
		resizeObserver = null;
	}
	observedContainer = null;
	lastChartWidth = 0;
	lastChartHeight = 0;
}

function observeChartContainer(container: HTMLElement) {
	if (!resizeObserver) {
		resizeObserver = new ResizeObserver(() => resizeChartToContainer());
	}
	if (observedContainer !== container) {
		resizeObserver.disconnect();
		resizeObserver.observe(container);
		observedContainer = container;
	}
}

function resizeChartToContainer() {
	const container = document.getElementById('container');
	if (!container || !chartInstance || chartInstance.isDisposed()) return;
	const width = container.clientWidth,
		height = container.clientHeight;
	if (width < 1 || height < 1 || (width === lastChartWidth && height === lastChartHeight)) return;
	chartInstance.resize({ width, height });
	lastChartWidth = width;
	lastChartHeight = height;
	buildCurrentRaster();
	chartInstance.setOption({
		grid: planeGrid(),
		graphic: rasterGraphic(),
		visualMap: buildVisualMap(
			get(previewState).quantity,
			get(previewState).unit,
			get(previewState).min,
			get(previewState).max
		)
	});
}

export function resizeECharts() {
	const container = document.getElementById('container');
	if (!container) return;
	observeChartContainer(container);
	resizeChartToContainer();
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
	const mesh = getFrameMesh();
	const cells = { x: mesh.dx, y: mesh.dy, z: mesh.dz };
	const counts = { x: mesh.Nx, y: mesh.Ny, z: mesh.Nz };
	const position = previewSampleCoordinateNm(
		ps.sliceIndex ?? ps.layer,
		counts[normal],
		cells[normal] * counts[normal] * 1e9
	);
	return `${normal}: ${formatDistanceNm(position)} nm · layer ${ps.sliceIndex ?? ps.layer}`;
}
