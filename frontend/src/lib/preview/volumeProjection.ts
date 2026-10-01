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
	const normal = ['x', 'y', 'z'].indexOf(axis),
		u = (normal + 1) % 3,
		v = (normal + 2) % 3;
	const sums = new Float64Array(dimensions[u] * dimensions[v] * 3);
	const count = Math.min(positions.length, values.length) / 3;
	const key = (i: number) => {
		const p = [
			positions[i * 3],
			positions[i * 3 + 1],
			Math.floor(positions[i * 3 + 2] / Math.max(stride, 1))
		];
		return (p[v] * dimensions[u] + p[u]) * 3;
	};
	for (let i = 0; i < count; i++) {
		const offset = key(i);
		for (let c = 0; c < 3; c++)
			sums[offset + c] +=
				values[i * 3 + c] *
				(normal === 2
					? Math.max(
							0,
							Math.min(
								stride,
								sourceDepth - Math.floor(positions[i * 3 + 2] / Math.max(stride, 1)) * stride
							)
						)
					: 1);
	}
	const result = new Float32Array(values.length),
		divisor = Math.max(normal === 2 ? sourceDepth : dimensions[normal], 1);
	for (let i = 0; i < count; i++) {
		const offset = key(i);
		for (let c = 0; c < 3; c++) result[i * 3 + c] = sums[offset + c] / divisor;
	}
	return result;
}
