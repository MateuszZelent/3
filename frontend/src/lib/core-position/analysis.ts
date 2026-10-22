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

export function validCoreRow(row: number[]): boolean {
	return (
		row.length >= 3 && row.slice(0, 3).every(Number.isFinite) && (row.length < 4 || row[3] === 1)
	);
}

// Analytics use the latest contiguous segment only. Invalid samples, repeated
// times and time resets break the segment instead of inventing an orbit.
export function analyzeCore(rows: number[][], windowNs: number) {
	const valid = rows.filter(validCoreRow);
	const last = rows.at(-1);
	let recent: number[][] = [];
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
		for (const row of recent.slice(1)) {
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
