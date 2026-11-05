export type ScalarHeatmapPoint = readonly number[];
export type ScalarWindow = { u: readonly [number, number]; v: readonly [number, number] };

export interface ScalarHeatmapRaster {
	width: number;
	height: number;
	pixels: Uint8ClampedArray;
	getSourceIndex(x: number, y: number): number;
}

const MAX_DIRECT_LOOKUP_CELLS = 4_000_000;
const MAX_FULL_RASTER_CELLS = 4_000_000;
const MAX_CANVAS_AXIS = 32_767;

function parseColor(color: string): [number, number, number] {
	const hex = color.startsWith('#') ? color.slice(1) : color;
	if (hex.length !== 6) return [0, 0, 0];
	return [
		Number.parseInt(hex.slice(0, 2), 16),
		Number.parseInt(hex.slice(2, 4), 16),
		Number.parseInt(hex.slice(4, 6), 16)
	];
}

function paintValue(
	value: number,
	min: number,
	max: number,
	colors: Array<[number, number, number]>,
	out: Uint8ClampedArray,
	pixelWidth: number,
	xStart: number,
	xEnd: number,
	yStart: number,
	yEnd: number
) {
	let red = 107,
		green = 122,
		blue = 154,
		alpha = 46;
	if (value >= min && value <= max && Number.isFinite(value)) {
		const t = max > min ? (value - min) / (max - min) : 0.5;
		const palettePosition = Math.max(0, Math.min(colors.length - 1, t * (colors.length - 1)));
		const lower = Math.floor(palettePosition);
		const upper = Math.min(colors.length - 1, lower + 1);
		const amount = palettePosition - lower;
		red = colors[lower][0] * (1 - amount) + colors[upper][0] * amount;
		green = colors[lower][1] * (1 - amount) + colors[upper][1] * amount;
		blue = colors[lower][2] * (1 - amount) + colors[upper][2] * amount;
		alpha = 255;
	}

	for (let y = yStart; y < yEnd; y++) {
		let offset = (y * pixelWidth + xStart) * 4;
		for (let x = xStart; x < xEnd; x++) {
			out[offset] = red;
			out[offset + 1] = green;
			out[offset + 2] = blue;
			out[offset + 3] = alpha;
			offset += 4;
		}
	}
}

/**
 * Rasterize every received cell once into a transparent image. Large domains
 * use viewport-sized output pixels to bound memory; every source tuple remains
 * addressable through getSourceIndex and is never altered or averaged.
 */
export function buildScalarHeatmapRaster(
	points: readonly ScalarHeatmapPoint[],
	domainWidth: number,
	domainHeight: number,
	min: number,
	max: number,
	palette: readonly string[],
	options: {
		window?: ScalarWindow;
		viewportWidth?: number;
		viewportHeight?: number;
		pixelRatio?: number;
	} = {}
): ScalarHeatmapRaster {
	const width = Math.max(1, Math.floor(domainWidth));
	const height = Math.max(1, Math.floor(domainHeight));
	const domainCells = width * height;
	const fullRaster =
		domainCells <= MAX_FULL_RASTER_CELLS && width <= MAX_CANVAS_AXIS && height <= MAX_CANVAS_AXIS;
	const pixelRatio = Math.max(1, Math.min(options.pixelRatio ?? 1, 2));
	const rasterWidth = fullRaster
		? width
		: Math.max(
				1,
				Math.min(width, Math.ceil((options.viewportWidth ?? 800) * pixelRatio), MAX_CANVAS_AXIS)
			);
	const rasterHeight = fullRaster
		? height
		: Math.max(
				1,
				Math.min(height, Math.ceil((options.viewportHeight ?? 600) * pixelRatio), MAX_CANVAS_AXIS)
			);
	const window = options.window ?? { u: [0, 1] as const, v: [0, 1] as const };
	const uSpan = Math.max(window.u[1] - window.u[0], Number.EPSILON);
	const vSpan = Math.max(window.v[1] - window.v[0], Number.EPSILON);
	const colors = palette.length
		? palette.map(parseColor)
		: ([
				[0, 0, 0],
				[255, 255, 255]
			] as Array<[number, number, number]>);
	const pixels = new Uint8ClampedArray(rasterWidth * rasterHeight * 4);
	let directLookup: Int32Array | undefined;
	let sortedKeys: Float64Array | undefined;
	let sortedIndices: Uint32Array | undefined;
	let unsortedLookup: Map<number, number> | undefined;
	if (domainCells <= MAX_DIRECT_LOOKUP_CELLS) {
		directLookup = new Int32Array(domainCells).fill(-1);
	} else {
		sortedKeys = new Float64Array(points.length);
		sortedIndices = new Uint32Array(points.length);
	}
	let monotonic = true;
	let previousKey = -Infinity;
	let validCount = 0;

	for (let index = 0; index < points.length; index++) {
		const point = points[index];
		const x = point?.[0],
			y = point?.[1],
			value = point?.[2];
		if (!Number.isInteger(x) || !Number.isInteger(y) || x < 0 || y < 0 || x >= width || y >= height)
			continue;
		const cellKey = y * width + x;
		if (directLookup) directLookup[cellKey] = index;
		else if (sortedKeys) {
			sortedKeys[validCount] = cellKey;
			sortedIndices![validCount] = index;
			if (cellKey < previousKey) monotonic = false;
			previousKey = cellKey;
		}
		validCount++;

		const normalizedU = (x + 0.5) / width;
		const normalizedV = (y + 0.5) / height;
		if (
			normalizedU < window.u[0] ||
			normalizedU > window.u[1] ||
			normalizedV < window.v[0] ||
			normalizedV > window.v[1]
		)
			continue;
		const cellStartU = Math.max(x / width, window.u[0]);
		const cellEndU = Math.min((x + 1) / width, window.u[1]);
		const cellStartV = Math.max(y / height, window.v[0]);
		const cellEndV = Math.min((y + 1) / height, window.v[1]);
		const pixelStartU = ((cellStartU - window.u[0]) / uSpan) * rasterWidth;
		const pixelEndU = ((cellEndU - window.u[0]) / uSpan) * rasterWidth;
		const pixelStartV = ((cellStartV - window.v[0]) / vSpan) * rasterHeight;
		const pixelEndV = ((cellEndV - window.v[0]) / vSpan) * rasterHeight;
		const roundingGuard = 1e-9;
		const xStart = Math.max(0, Math.floor(pixelStartU + roundingGuard));
		const xEnd = Math.min(rasterWidth, Math.ceil(pixelEndU - roundingGuard));
		const yStart = Math.max(0, rasterHeight - Math.ceil(pixelEndV - roundingGuard));
		const yEnd = Math.min(rasterHeight, rasterHeight - Math.floor(pixelStartV + roundingGuard));
		if (xStart >= xEnd || yStart >= yEnd) continue;
		paintValue(value, min, max, colors, pixels, rasterWidth, xStart, xEnd, yStart, yEnd);
	}

	if (directLookup) {
		// The raster already preserves input-order overwrite semantics for duplicate cells.
	} else if (sortedKeys && !monotonic) {
		unsortedLookup = new Map<number, number>();
		for (let index = 0; index < points.length; index++) {
			const point = points[index];
			if (
				Number.isInteger(point?.[0]) &&
				Number.isInteger(point?.[1]) &&
				point[0] >= 0 &&
				point[1] >= 0 &&
				point[0] < width &&
				point[1] < height
			) {
				unsortedLookup.set(point[1] * width + point[0], index);
			}
		}
		sortedKeys = undefined;
		sortedIndices = undefined;
	} else if (sortedKeys && sortedIndices) {
		sortedKeys = sortedKeys.subarray(0, validCount);
		sortedIndices = sortedIndices.subarray(0, validCount);
	}

	const getSourceIndex = (x: number, y: number) => {
		if (!Number.isInteger(x) || !Number.isInteger(y) || x < 0 || y < 0 || x >= width || y >= height)
			return -1;
		const key = y * width + x;
		if (directLookup) return directLookup[key];
		if (unsortedLookup) return unsortedLookup.get(key) ?? -1;
		if (sortedKeys && sortedIndices) {
			let low = 0;
			let high = sortedKeys.length;
			while (low < high) {
				const middle = (low + high) >>> 1;
				if (sortedKeys[middle] <= key) low = middle + 1;
				else high = middle;
			}
			return low > 0 && sortedKeys[low - 1] === key ? sortedIndices[low - 1] : -1;
		}
		return -1;
	};

	return { width: rasterWidth, height: rasterHeight, pixels, getSourceIndex };
}
