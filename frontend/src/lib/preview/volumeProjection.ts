export type ProjectionAxis = 'x' | 'y' | 'z';
// Means include zero values and absent cells, matching the full-depth 2D reduction.
export function projectVolumeVectors(
	positions: Int32Array,
	values: Float32Array,
	dimensions: [number, number, number],
	stride: number,
	axis: ProjectionAxis,
	sourceDepth = dimensions[2] * Math.max(stride, 1)
): Float32Array {
	const normal = axis === 'x' ? 0 : axis === 'y' ? 1 : 2;
	const u = (normal + 1) % 3;
	const v = (normal + 2) % 3;
	const safeStride = Math.max(Math.floor(stride), 1);
	const uSize = Math.max(Math.floor(dimensions[u]), 0);
	const vSize = Math.max(Math.floor(dimensions[v]), 0);
	const sums = new Float64Array(uSize * vSize * 3);
	const count = Math.min(Math.floor(positions.length / 3), Math.floor(values.length / 3));
	const positionOffset = (index: number, coordinate: number) =>
		coordinate === 2
			? Math.floor(positions[index * 3 + 2] / safeStride)
			: positions[index * 3 + coordinate];
	for (let i = 0; i < count; i++) {
		const uPosition = positionOffset(i, u),
			vPosition = positionOffset(i, v);
		if (uPosition < 0 || uPosition >= uSize || vPosition < 0 || vPosition >= vSize) continue;
		const offset = (vPosition * uSize + uPosition) * 3;
		const zBin = Math.floor(positions[i * 3 + 2] / safeStride);
		const thickness = Math.max(0, Math.min(safeStride, sourceDepth - zBin * safeStride));
		const weight = normal === 2 ? thickness : 1;
		for (let c = 0; c < 3; c++) sums[offset + c] += values[i * 3 + c] * weight;
	}
	const result = new Float32Array(values.length);
	const divisor = Math.max(normal === 2 ? sourceDepth : dimensions[normal], 1);
	for (let i = 0; i < count; i++) {
		const uPosition = positionOffset(i, u),
			vPosition = positionOffset(i, v);
		if (uPosition < 0 || uPosition >= uSize || vPosition < 0 || vPosition >= vSize) continue;
		const offset = (vPosition * uSize + uPosition) * 3;
		for (let c = 0; c < 3; c++) result[i * 3 + c] = sums[offset + c] / divisor;
	}
	return result;
}
