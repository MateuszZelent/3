import { get } from 'svelte/store';
import { post, type PostResult } from '$api/post';
import { previewControlsBusy } from '$api/previewTransition';
import { setAlert } from '$lib/alerts/alert';

type PreviewRequest = {
	endpoint: string;
	data: Record<string, unknown>;
	key: string;
	merge?: 'replace' | 'object';
	waiters: Array<(result: PostResult) => void>;
	controller?: AbortController;
	cancelled?: boolean;
};

const clientId = Math.random().toString(36).slice(2);
let nextRequestId = 0;
let activeRequest: PreviewRequest | null = null;
const pendingRequests: PreviewRequest[] = [];
const idleWaiters = new Set<() => void>();
const SIZE_KEYS = new Set(['xSize', 'ySize', 'zSize', 'planeResolution']);

function isAck(data: unknown, requestId: string): boolean {
	if (!data || typeof data !== 'object') return false;
	const ack = data as { requestId?: unknown; configRevision?: unknown };
	return (
		ack.requestId === requestId &&
		typeof ack.configRevision === 'number' &&
		Number.isSafeInteger(ack.configRevision) &&
		ack.configRevision >= 0
	);
}

function addWaiter(request: PreviewRequest): Promise<PostResult> {
	return new Promise((resolve) => request.waiters.push(resolve));
}

function blockedResult(): PostResult {
	return { ok: false, error: 'Preview controls are unavailable during a view transition' };
}

function resolveWaiters(request: PreviewRequest, result: PostResult) {
	for (const resolve of request.waiters) resolve(result);
}

function resolveIdleWaiters() {
	if (activeRequest || pendingRequests.length > 0) return;
	for (const resolve of idleWaiters) resolve();
	idleWaiters.clear();
}

function replacePending(request: PreviewRequest, index: number) {
	const [old] = pendingRequests.splice(index, 1);
	if (request.merge === 'object' && old.merge === 'object') {
		request.data = { ...old.data, ...request.data };
	}
	request.waiters.unshift(...old.waiters);
	pendingRequests.push(request);
}

function enqueue(
	endpoint: string,
	data: Record<string, unknown>,
	key: string,
	merge: PreviewRequest['merge'] = 'replace'
): Promise<PostResult> {
	if (get(previewControlsBusy)) return Promise.resolve(blockedResult());

	const request: PreviewRequest = { endpoint, data: { ...data }, key, merge, waiters: [] };
	const result = addWaiter(request);
	if (key === 'fullResolution') {
		for (let index = pendingRequests.length - 1; index >= 0; index--) {
			if (!SIZE_KEYS.has(pendingRequests[index].key)) continue;
			const [superseded] = pendingRequests.splice(index, 1);
			request.waiters.unshift(...superseded.waiters);
		}
	}
	const existingIndex = pendingRequests.findIndex((pending) => pending.key === key);
	if (existingIndex >= 0) {
		replacePending(request, existingIndex);
	} else {
		pendingRequests.push(request);
	}
	drain();
	return result;
}

function drain() {
	if (activeRequest || pendingRequests.length === 0) return;
	const request = pendingRequests.shift();
	if (!request) return;
	activeRequest = request;
	request.controller = new AbortController();
	const requestId = `preview-control-${clientId}-${++nextRequestId}`;
	void post(
		`preview/${request.endpoint}`,
		{ ...request.data, requestId },
		{ parseResponse: true, signal: request.controller.signal }
	)
		.then((result) => {
			if (request.cancelled) return;
			if (result.ok && !isAck(result.data, requestId)) {
				const error = 'Preview update returned an invalid config acknowledgement';
				setAlert(error);
				resolveWaiters(request, {
					ok: false,
					error
				});
			} else {
				resolveWaiters(request, result);
			}
		})
		.catch((error: unknown) => {
			if (request.cancelled) return;
			resolveWaiters(request, {
				ok: false,
				error: error instanceof Error ? error.message : String(error)
			});
		})
		.finally(() => {
			if (activeRequest !== request) return;
			activeRequest = null;
			if (pendingRequests.length > 0) drain();
			else resolveIdleWaiters();
		});
}

export function cancelPreviewMutations() {
	const result: PostResult = { ok: false, error: 'Preview controls were closed' };
	if (activeRequest) {
		activeRequest.cancelled = true;
		activeRequest.controller?.abort();
		resolveWaiters(activeRequest, result);
		activeRequest = null;
	}
	for (const request of pendingRequests.splice(0)) resolveWaiters(request, result);
	resolveIdleWaiters();
}

export function waitForPreviewMutations(): Promise<void> {
	if (!activeRequest && pendingRequests.length === 0) return Promise.resolve();
	return new Promise((resolve) => idleWaiters.add(resolve));
}

export function postComponent(component: string, options: { allLayers?: boolean } = {}) {
	return enqueue('component', { component, ...options }, 'component');
}

export function postQuantity(quantity: string) {
	return enqueue('quantity', { quantity }, 'quantity');
}

export function postLayer(layer: number) {
	return enqueue('layer', { layer }, 'layer');
}

export function postXChosenSize(xChosenSize: number) {
	return enqueue('XChosenSize', { xChosenSize }, 'xSize');
}
export function postYChosenSize(yChosenSize: number) {
	return enqueue('YChosenSize', { yChosenSize }, 'ySize');
}

export function postRefresh() {
	return enqueue('refresh', {}, 'refresh');
}

export function postAllLayers(allLayers: boolean) {
	return enqueue('allLayers', { allLayers }, 'allLayers');
}

export function postAutoScaleEnabled(autoScaleEnabled: boolean) {
	return enqueue('autoScaleEnabled', { autoScaleEnabled }, 'autoScaleEnabled');
}

export function postZChosenSize(zChosenSize: number) {
	return enqueue('ZChosenSize', { zChosenSize }, 'zSize');
}

export function postMaxPoints(maxPoints: number) {
	return enqueue('maxpoints', { maxPoints }, 'maxPoints');
}
export function postScale(scale: number) {
	return enqueue('scale', { scale }, 'scale');
}

export function postFullResolution() {
	return enqueue('fullResolution', {}, 'fullResolution');
}

export function postSection(section: {
	plane?: 'xy' | 'yz' | 'xz';
	mode?: 'single' | 'average';
	sliceIndex?: number;
}) {
	return enqueue('section', section, 'section', 'object');
}
export function postPlaneResolution(resolution: { uSize?: number; vSize?: number }) {
	return enqueue('planeResolution', resolution, 'planeResolution', 'object');
}
