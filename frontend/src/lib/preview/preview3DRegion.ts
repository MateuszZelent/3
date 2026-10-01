import { get, writable } from 'svelte/store';
import { previewState } from '$api/incoming/preview';
import { meshState } from '$api/incoming/mesh';
import { setPreviewClientBudget } from '$api/websocket';
export type CellTriple = [number, number, number];
export interface PreviewRegion {
	enabled: boolean;
	start: CellTriple;
	end: CellTriple;
	mode: 'native' | 'custom';
	samples: CellTriple;
	maxPoints: number;
	requestId: string;
}
export const regionDraft = writable<PreviewRegion>({
	enabled: false,
	start: [0, 0, 0],
	end: [1, 1, 1],
	mode: 'native',
	samples: [1, 1, 1],
	maxPoints: 1000000,
	requestId: ''
});
export const regionPending = writable(false);
export const regionError = writable('');
let timer: ReturnType<typeof setTimeout> | undefined,
	inFlight = false,
	revision = 0,
	acknowledged = 0;
let lastMesh = '';
const session = Math.random().toString(36).slice(2);
export function cellWindow(range: [number, number], count: number): [number, number] {
	const width = Math.max(1, Math.min(count, Math.round((range[1] - range[0]) * count)));
	const start = Math.max(0, Math.min(count - width, Math.round(range[0] * count)));
	return [start, start + width];
}
previewState.subscribe((p) => {
	if (!inFlight && revision === acknowledged && p.region?.mode) regionDraft.set(p.region);
});
meshState.subscribe((m) => {
	const counts: CellTriple = [Math.max(m.Nx, 1), Math.max(m.Ny, 1), Math.max(m.Nz, 1)];
	const key = counts.join('/');
	if (key === lastMesh) return;
	lastMesh = key;
	if (!get(regionDraft).enabled)
		regionDraft.update((r) => ({ ...r, start: [0, 0, 0], end: counts, samples: counts }));
});
export function changeRegion(patch: Partial<PreviewRegion>) {
	regionDraft.update((r) => ({ ...r, ...patch }));
	revision++;
	regionPending.set(true);
	regionError.set('');
	if (get(regionDraft).enabled) setPreviewClientBudget(1000000);
	clearTimeout(timer);
	timer = setTimeout(sendLatest, 120);
}
async function sendLatest() {
	if (inFlight || revision === acknowledged) return;
	inFlight = true;
	const sending = revision;
	const draft = { ...get(regionDraft), requestId: `${session}-${sending}` };
	try {
		const response = await fetch('./api/preview/region', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(draft)
		});
		if (!response.ok) {
			const error = await response.json();
			throw new Error(error.error || 'Unable to update render window');
		}
		const applied = (await response.json()) as PreviewRegion;
		acknowledged = sending;
		if (revision === sending) {
			regionDraft.set(applied);
			regionPending.set(false);
		}
	} catch (error) {
		acknowledged = sending;
		if (revision === sending) {
			regionError.set(String(error));
			regionPending.set(false);
		}
	} finally {
		inFlight = false;
		if (revision !== acknowledged) void sendLatest();
	}
}
export function setRegionWindow(axis: number, range: [number, number]) {
	const m = get(meshState),
		count = [m.Nx, m.Ny, m.Nz][axis];
	if (count < 1) return;
	const r = get(regionDraft),
		start = [...r.start] as CellTriple,
		end = [...r.end] as CellTriple;
	[start[axis], end[axis]] = cellWindow(range, count);
	changeRegion({ enabled: true, start, end });
}
export function fullDomain() {
	changeRegion({ enabled: false });
}
