export type ColorScale = { min: number; max: number; palette: string[] };

export function getColorScale(min: number, max: number): ColorScale {
	// CUDA preview averaging uses Float32. Do not stretch a few rounding ULPs
	// of a constant field into the entire palette (e.g. uniform alpha).
	const magnitude = Math.max(Math.abs(min), Math.abs(max));
	if (Number.isFinite(min) && Number.isFinite(max) && max - min <= magnitude * 1e-6) {
		const center = (min + max) / 2;
		const padding = magnitude > 0 ? magnitude * 0.05 : 1e-12;
		min = center - padding;
		max = center + padding;
	}

	if (min < 0 && max > 0) {
		const bound = Math.max(Math.abs(min), Math.abs(max));
		return {
			min: -bound,
			max: bound,
			palette: ['#15315f', '#2f6caa', '#90b9df', '#f4f1ed', '#efb09d', '#cf6256', '#7d1d34']
		};
	}

	if (max <= 0) {
		return {
			min,
			max,
			palette: ['#f3f7fd', '#cfdef1', '#91b8dd', '#5688bd', '#285b93', '#14365f']
		};
	}

	return {
		min,
		max,
		palette: ['#0a1220', '#143d67', '#1c6d8f', '#24a0a4', '#8ed6ac', '#f1f7bb']
	};
}
