export type NumericFieldComponent = 'magnitude' | 'value' | 'x' | 'y' | 'z';
export type NumericFieldRange = Readonly<{ min: number; max: number }>;

type CacheEntry = Map<string, NumericFieldRange>;
type OccupancyCache = Map<Uint8Array | undefined, CacheEntry>;
const rangesByValues = new WeakMap<Float32Array, OccupancyCache>();

function sampleValue(values: Float32Array, offset: number, component: NumericFieldComponent) {
	const x = values[offset],
		y = values[offset + 1],
		z = values[offset + 2];
	switch (component) {
		case 'magnitude':
			return Math.hypot(x, y, z);
		case 'y':
			return y;
		case 'z':
			return z;
		case 'value':
		case 'x':
			return x;
	}
}

/**
 * Cache ranges by immutable wire-buffer identity plus every value-affecting
 * selector. Incoming MessagePack frames own fresh typed arrays, so this avoids
 * rescanning a frame when only its palette/color component changes.
 */
export function getNumericFieldRange(
	values: Float32Array,
	component: NumericFieldComponent,
	occupancy: Uint8Array | undefined,
	vectorCount: number,
	normScale: number,
	dataRevision: number | undefined
): NumericFieldRange {
	let byOccupancy = rangesByValues.get(values);
	if (!byOccupancy) {
		byOccupancy = new Map();
		rangesByValues.set(values, byOccupancy);
	}
	let byKey = byOccupancy.get(occupancy);
	if (!byKey) {
		byKey = new Map();
		byOccupancy.set(occupancy, byKey);
	}
	const count = Math.max(0, Math.min(Math.floor(vectorCount), Math.floor(values.length / 3)));
	const key = `${component}/${count}/${normScale}/${dataRevision ?? ''}`;
	const cached = byKey.get(key);
	if (cached) return cached;

	let min = Infinity;
	let max = -Infinity;
	for (let index = 0; index < count; index++) {
		if (occupancy?.[index] === 0) continue;
		const value = sampleValue(values, index * 3, component) * normScale;
		if (!Number.isFinite(value)) continue;
		if (value < min) min = value;
		if (value > max) max = value;
	}
	const range = Number.isFinite(min) ? { min, max } : { min: 0, max: 1 };
	byKey.set(key, range);
	return range;
}
