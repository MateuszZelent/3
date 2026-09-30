/**
 * Returns the physical coordinate of a rendered preview sample.
 *
 * The preview kernels sample the full mesh into `sampleCount` equally sized bins.
 * A sample belongs at the centre of its bin, not on an outer mesh boundary.
 */
export function previewSampleCoordinateNm(
	sampleIndex: number,
	sampleCount: number,
	extentNm: number
): number {
	if (!Number.isFinite(sampleIndex) || !Number.isFinite(sampleCount) || sampleCount < 1) {
		return Number.NaN;
	}

	return ((sampleIndex + 0.5) * extentNm) / sampleCount;
}

export type PreviewPlane = 'xy' | 'yz' | 'xz';
export type PreviewAxis = 'x' | 'y' | 'z';

export function previewPlaneAxes(plane: PreviewPlane = 'xy'): {
	u: PreviewAxis;
	v: PreviewAxis;
	normal: PreviewAxis;
} {
	switch (plane) {
		case 'yz':
			return { u: 'y', v: 'z', normal: 'x' };
		case 'xz':
			return { u: 'x', v: 'z', normal: 'y' };
		default:
			return { u: 'x', v: 'y', normal: 'z' };
	}
}

// Fit the full physical extent inside the plotting area without stretching cells.
export function fitPreviewPlane(width: number, height: number, extentU: number, extentV: number) {
	const ratio = extentU > 0 && extentV > 0 ? extentU / extentV : 1;
	const plotWidth = Math.min(Math.max(width, 1), Math.max(height, 1) * ratio);
	const plotHeight = plotWidth / ratio;
	return {
		width: plotWidth,
		height: plotHeight,
		left: (width - plotWidth) / 2,
		top: (height - plotHeight) / 2
	};
}
