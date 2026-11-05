export interface CoreMetrics {
	radiusNm: number;
	centerXNm: number;
	centerYNm: number;
	frequencyGHz: number | null;
	totalPoints: number;
	orbitPoints: number;
	hasData: boolean;
}

export const emptyMetrics: CoreMetrics = {
	radiusNm: 0,
	centerXNm: 0,
	centerYNm: 0,
	frequencyGHz: null,
	totalPoints: 0,
	orbitPoints: 0,
	hasData: false
};

export interface CorePlotCounts {
	sourcePoints: number;
	renderedPoints: number;
	sampled: boolean;
}

export function getCorePlotCounts(
	validPoints: number,
	orbitPoints: number,
	limit = 5_000
): CorePlotCounts {
	const sourcePoints = Number.isFinite(validPoints) ? Math.max(0, Math.floor(validPoints)) : 0;
	const recentPoints = Number.isFinite(orbitPoints)
		? Math.min(sourcePoints, Math.max(0, Math.floor(orbitPoints)))
		: 0;
	const historyPoints = sourcePoints - recentPoints;
	const maxPoints = Number.isFinite(limit) ? Math.max(1, Math.floor(limit)) : 5_000;
	const renderedPoints = Math.min(historyPoints, maxPoints) + Math.min(recentPoints, maxPoints);
	return { sourcePoints, renderedPoints, sampled: renderedPoints < sourcePoints };
}

export function validCoreRow(row: number[]): boolean {
	return (
		row.length >= 3 &&
		Number.isFinite(row[0]) &&
		Number.isFinite(row[1]) &&
		Number.isFinite(row[2]) &&
		(row.length < 4 || row[3] === 1)
	);
}

export function sampleCoreCoordinates(
	rows: number[][],
	start: number,
	end: number,
	limit = 5_000
): number[][] {
	const count = Math.max(0, Math.min(rows.length, end) - Math.max(0, start));
	const first = Math.max(0, start);
	const maxPoints = Number.isFinite(limit) ? Math.max(1, Math.floor(limit)) : 5_000;
	const sampleCount = Math.min(count, maxPoints);
	const points: number[][] = [];
	for (let sample = 0; sample < sampleCount; sample++) {
		const offset =
			sampleCount === 1 ? count - 1 : Math.floor((sample * (count - 1)) / (sampleCount - 1));
		const row = rows[first + offset];
		points.push([row[1], row[2]]);
	}
	return points;
}

// Analytics use the latest contiguous segment only. Invalid samples, repeated
// times and time resets break the segment instead of inventing an orbit.
export function analyzeCore(rows: number[][], windowNs: number) {
	const valid = rows.filter(validCoreRow);
	const last = rows.at(-1);
	const recent: number[][] = [];
	if (last && validCoreRow(last)) {
		const cut = last[0] - Math.max(0.1, windowNs) * 1e-9;
		for (let i = rows.length - 1; i >= 0; i--) {
			const row = rows[i];
			if (
				!validCoreRow(row) ||
				row[0] < cut ||
				(recent.length && row[0] >= recent[recent.length - 1][0])
			)
				break;
			recent.push(row);
		}
		recent.reverse();
	}
	const metrics: CoreMetrics = {
		...emptyMetrics,
		totalPoints: rows.length,
		orbitPoints: recent.length
	};
	if (!recent.length) return { valid, recent, metrics };
	let cx = 0,
		cy = 0;
	for (const row of recent) {
		cx += row[1];
		cy += row[2];
	}
	cx /= recent.length;
	cy /= recent.length;
	let radius = 0;
	for (const row of recent) radius += Math.hypot(row[1] - cx, row[2] - cy);
	radius /= recent.length;
	metrics.centerXNm = cx * 1e9;
	metrics.centerYNm = cy * 1e9;
	metrics.radiusNm = radius * 1e9;
	metrics.hasData = true;
	if (recent.length >= 9 && radius > 1e-15) {
		let angle = 0,
			travel = 0,
			previous = Math.atan2(recent[0][2] - cy, recent[0][1] - cx);
		let sampled = true;
		for (let i = 1; i < recent.length; i++) {
			const row = recent[i];
			const current = Math.atan2(row[2] - cy, row[1] - cx);
			const delta = Math.atan2(Math.sin(current - previous), Math.cos(current - previous));
			if (Math.abs(delta) >= Math.PI / 2) sampled = false;
			angle += delta;
			travel += Math.abs(delta);
			previous = current;
		}
		const dt = recent[recent.length - 1][0] - recent[0][0];
		if (
			sampled &&
			dt > 0 &&
			Math.abs(angle) >= 0.99 * 2 * Math.PI &&
			Math.abs(angle) >= 0.8 * travel
		) {
			metrics.frequencyGHz = Math.abs(angle) / (2 * Math.PI * dt * 1e9);
		}
	}
	return { valid, recent, metrics };
}
