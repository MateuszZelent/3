/**
 * Returns the physical coordinate of a rendered preview sample.
 *
 * `cuda.Resize` samples the full mesh into `sampleCount` equally sized bins.
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
