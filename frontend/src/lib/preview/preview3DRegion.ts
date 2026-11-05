import { get, writable } from 'svelte/store';
import { previewState } from '$api/incoming/preview';
import { meshState } from '$api/incoming/mesh';
import { setPreviewClientBudget } from '$api/websocket';
import { post, type PostResult } from '$api/post';
import { previewControlsBusy } from '$api/previewTransition';
export type CellTriple = [number, number, number];
export interface PreviewRegion {
	enabled: boolean;
	start: CellTriple;
	end: CellTriple;
	mode: 'native' | 'custom';
	samples: CellTriple;
	maxPoints: number;
	requestId: string;
	configRevision?: number;
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
	inFlight: Promise<PostResult> | null = null,
	revision = 0,
	acknowledged = 0;
let lastMesh = '';
let generation = 0;
let activeController: AbortController | undefined;
const session = Math.random().toString(36).slice(2);
export function cellWindow(range: [number, number], count: number): [number, number] {
	const width = Math.max(1, Math.min(count, Math.round((range[1] - range[0]) * count)));
	const start = Math.max(0, Math.min(count - width, Math.round(range[0] * count)));
	return [start, start + width];
}
previewState.subscribe((p) => {
	if (inFlight || revision !== acknowledged || !p.region?.mode) return;
	// A frame produced before our latest acknowledged request must not roll the
	// draft back to an older window.
	const id = p.region.requestId || '';
	if (id.startsWith(`${session}-`) && Number(id.slice(session.length + 1)) < acknowledged) return;
	regionDraft.set(p.region);
});
meshState.subscribe((m) => {
	const counts: CellTriple = [Math.max(m.Nx, 1), Math.max(m.Ny, 1), Math.max(m.Nz, 1)];
	const key = counts.join('/');
	if (key === lastMesh) return;
	lastMesh = key;
	if (!get(regionDraft).enabled)
		regionDraft.update((r) => ({ ...r, start: [0, 0, 0], end: counts, samples: counts }));
});
export function changeRegion(patch: Partial<PreviewRegion>): boolean {
	if (get(previewControlsBusy)) return false;
	regionDraft.update((r) => ({ ...r, ...patch }));
	revision++;
	regionPending.set(true);
	regionError.set('');
	if (get(regionDraft).enabled) setPreviewClientBudget(1000000);
	clearTimeout(timer);
	timer = setTimeout(() => void sendLatest(), 120);
	return true;
}

function isAppliedRegion(data: unknown, requestId: string): data is PreviewRegion {
	if (!data || typeof data !== 'object') return false;
	const applied = data as Partial<PreviewRegion>;
	return (
		applied.requestId === requestId &&
		typeof applied.configRevision === 'number' &&
		Number.isSafeInteger(applied.configRevision) &&
		applied.configRevision >= 0 &&
		typeof applied.enabled === 'boolean' &&
		Array.isArray(applied.start) &&
		Array.isArray(applied.end) &&
		Array.isArray(applied.samples)
	);
}

async function drainLatest(token: number): Promise<PostResult> {
	let result: PostResult = { ok: true };
	while (token === generation && revision !== acknowledged) {
		const sending = revision;
		const requestId = `${session}-${sending}`;
		const draft = { ...get(regionDraft), requestId };
		activeController = new AbortController();
		result = await post('preview/region', draft, {
			parseResponse: true,
			alert: false,
			signal: activeController.signal
		});
		if (token !== generation) return { ok: false, error: 'Render window controls were closed' };
		activeController = undefined;
		if (result.ok && !isAppliedRegion(result.data, requestId)) {
			result = { ok: false, error: 'Region update returned an invalid config acknowledgement' };
		}
		acknowledged = sending;
		if (revision === sending) {
			if (result.ok) {
				regionDraft.set(result.data as PreviewRegion);
				regionError.set('');
			} else {
				regionError.set(result.error);
			}
			regionPending.set(false);
		}
	}
	return result;
}

function sendLatest(): Promise<PostResult> {
	clearTimeout(timer);
	if (revision === acknowledged) return Promise.resolve({ ok: true });
	if (inFlight) return inFlight;
	const token = generation;
	const pending = drainLatest(token).finally(() => {
		if (token !== generation) return;
		if (inFlight === pending) inFlight = null;
		if (revision !== acknowledged) void sendLatest();
	});
	inFlight = pending;
	return pending;
}

export function cancelRegionUpdates() {
	generation++;
	clearTimeout(timer);
	timer = undefined;
	activeController?.abort();
	activeController = undefined;
	inFlight = null;
	acknowledged = revision;
	regionPending.set(false);
}

export async function waitForRegionUpdate(): Promise<void> {
	clearTimeout(timer);
	while (revision !== acknowledged) {
		const result = await sendLatest();
		if (!result.ok && revision === acknowledged) throw new Error(result.error);
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
	const m = get(meshState);
	const counts: CellTriple = [Math.max(m.Nx, 1), Math.max(m.Ny, 1), Math.max(m.Nz, 1)];
	changeRegion({ enabled: false, start: [0, 0, 0], end: counts, samples: counts });
}
