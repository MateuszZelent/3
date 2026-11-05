import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { previewState } from './incoming/preview';
import { meshState } from './incoming/mesh';
import {
	cancelPreviewTransition,
	failPreviewTransition,
	finishPreviewTransition,
	markPreviewRendering,
	previewTransition,
	previewTransitionError,
	requestPreviewResync,
	requestPreviewTransition,
	retryPreviewTransition,
	setPreviewRecoveryHandler
} from './previewTransition';

const postMock = vi.hoisted(() => vi.fn());
vi.mock('./post', () => ({ post: postMock }));

function currentFrame(overrides: Record<string, unknown> = {}) {
	return { ...get(previewState), ...overrides };
}

async function flushMicrotasks() {
	await Promise.resolve();
	await Promise.resolve();
	await Promise.resolve();
}

function transition2D(timeoutMs = 10000) {
	return requestPreviewTransition(
		'component',
		{ component: 'x' },
		'Switching to 2D section',
		{ type: '2D' },
		{ timeoutMs }
	);
}

describe('preview transition recovery', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		previewTransition.set(null);
		previewTransitionError.set('');
		previewState.update((state) => ({
			...state,
			type: '3D',
			quantity: 'm',
			component: '3D',
			sequence: 10,
			configRevision: 1
		}));
		finishPreviewTransition(get(previewState) as never);
		postMock.mockReset().mockImplementation(async (_endpoint, data) => ({
			ok: true,
			data: { requestId: data.requestId, configRevision: 2 }
		}));
		vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
			callback(0);
			return 1;
		});
	});
	afterEach(() => {
		cancelPreviewTransition({ restore: false });
		setPreviewRecoveryHandler(null);
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});

	it('clears a 2D/3D transition when HTTP succeeds but no matching frame arrives by the deadline', async () => {
		const request = transition2D();
		await flushMicrotasks();
		await request;

		expect(get(previewTransition)).not.toBeNull();
		await vi.advanceTimersByTimeAsync(10000);

		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toMatch(/timed out|timeout/i);
		expect(postMock).toHaveBeenCalledOnce();
		const [endpoint, body, options] = postMock.mock.calls[0];
		expect(endpoint).toBe('preview/component');
		expect(body.component).toBe('x');
		expect(body.requestId).toEqual(expect.any(String));
		expect(options).toMatchObject({ timeoutMs: 10000, alert: false, parseResponse: true });
		expect(options.signal).toBeInstanceOf(AbortSignal);
	});

	it('clears the timeout error after a matching late recovery frame is rendered', async () => {
		const lastGood = currentFrame({ type: '3D', sequence: 10, configRevision: 1 });
		finishPreviewTransition(lastGood as never);
		const recovery = vi.fn();
		setPreviewRecoveryHandler(recovery);

		const request = transition2D(25);
		await flushMicrotasks();
		await request;
		await vi.advanceTimersByTimeAsync(25);

		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toMatch(/timed out|timeout/i);
		expect(recovery).toHaveBeenCalledOnce();

		const staleRevision = currentFrame({
			type: '2D',
			component: 'x',
			sequence: 11,
			configRevision: 1
		});
		expect(finishPreviewTransition(staleRevision as never)).toBe(false);
		expect(get(previewTransitionError)).toMatch(/timed out|timeout/i);

		const recovered = currentFrame({
			type: '2D',
			component: 'x',
			sequence: 12,
			configRevision: 2
		});
		expect(finishPreviewTransition(recovered as never)).toBe(true);
		expect(get(previewTransitionError)).toBe('');
		expect(retryPreviewTransition()).toBeUndefined();
	});

	it('waits for the exact config revision, target fields, and completed render', async () => {
		const request = transition2D();
		await flushMicrotasks();
		await request;
		const transition = get(previewTransition);
		expect(transition?.expectedRevision).toBe(2);

		const staleRevision = currentFrame({
			type: '2D',
			component: 'x',
			sequence: 11,
			configRevision: 1
		});
		expect(markPreviewRendering(staleRevision as never)).toBe(false);
		expect(finishPreviewTransition(staleRevision as never)).toBe(false);
		expect(get(previewTransition)?.id).toBe(transition?.id);

		const wrongTarget = currentFrame({ type: '3D', sequence: 12, configRevision: 2 });
		expect(markPreviewRendering(wrongTarget as never)).toBe(false);
		expect(finishPreviewTransition(wrongTarget as never)).toBe(false);
		expect(get(previewTransition)?.id).toBe(transition?.id);

		const matching = currentFrame({
			type: '2D',
			component: 'x',
			sequence: 13,
			configRevision: 2
		});
		expect(markPreviewRendering(matching as never)).toBe(true);
		expect(get(previewTransition)?.phase).toBe('rendering');
		expect(get(previewTransition)?.id).toBe(transition?.id);
		expect(finishPreviewTransition(matching as never)).toBe(true);
		expect(get(previewTransition)).toBeNull();
	});

	it('completes on a later config revision when the server skipped the acknowledged one', async () => {
		// A finished console operation can bump the revision before the capture
		// at the acknowledged revision is published; that frame is never sent.
		const request = transition2D();
		await flushMicrotasks();
		await request;
		expect(get(previewTransition)?.expectedRevision).toBe(2);

		const later = currentFrame({ type: '2D', component: 'x', sequence: 11, configRevision: 3 });
		expect(finishPreviewTransition(later as never)).toBe(true);
		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toBe('');
	});

	it('completes when the matching frame rendered before its HTTP acknowledgement arrives', async () => {
		let resolvePost!: (result: unknown) => void;
		postMock.mockReturnValueOnce(new Promise((resolve) => (resolvePost = resolve)));
		const request = transition2D();
		await flushMicrotasks();

		const matching = currentFrame({
			type: '2D',
			component: 'x',
			sequence: 11,
			configRevision: 2
		});
		expect(finishPreviewTransition(matching as never)).toBe(false);
		expect(get(previewTransition)).not.toBeNull();

		resolvePost({
			ok: true,
			data: { requestId: postMock.mock.calls[0][1].requestId, configRevision: 2 }
		});
		await request;
		expect(get(previewTransition)).toBeNull();
	});

	it('restores the last rendered frame and resyncs after a render or connection failure', async () => {
		const lastGood = currentFrame({ type: '3D', sequence: 10, configRevision: 1 });
		finishPreviewTransition(lastGood as never);
		const recovery = vi.fn();
		setPreviewRecoveryHandler(recovery);

		const request = transition2D();
		await flushMicrotasks();
		await request;
		previewState.set(
			currentFrame({ type: '2D', component: 'x', sequence: 11, configRevision: 2 }) as never
		);
		expect(failPreviewTransition(new Error('Preview disconnected'))).toBe(true);
		expect(get(previewState)).toBe(lastGood);
		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toBe('Preview disconnected');
		expect(recovery).toHaveBeenCalledOnce();
	});

	it('restores the successful frame geometry without rolling back the shared mesh store', async () => {
		const frameMesh = {
			Nx: 4,
			Ny: 5,
			Nz: 6,
			dx: 1,
			dy: 2,
			dz: 3,
			Tx: 1,
			Ty: 1,
			Tz: 1,
			PBCx: 0,
			PBCy: 0,
			PBCz: 0
		};
		const latestMesh = { ...frameMesh, Nx: 12, Ny: 10, Nz: 8 };
		const lastGood = currentFrame({ mesh: frameMesh, type: '3D', sequence: 10, configRevision: 1 });
		meshState.set(latestMesh);
		finishPreviewTransition(lastGood as never);
		const request = transition2D();
		await flushMicrotasks();
		await request;

		expect(failPreviewTransition(new Error('render failed'))).toBe(true);
		expect(get(previewState)).toBe(lastGood);
		expect(get(previewState).mesh).toBe(frameMesh);
		expect(get(meshState)).toBe(latestMesh);
	});

	it('clears busy and records a network failure for a user retry', async () => {
		postMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));
		await transition2D();

		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toBe('Failed to fetch');
		const retry = retryPreviewTransition();
		expect(retry).toBeDefined();
		await flushMicrotasks();
		await retry;
		expect(get(previewTransition)?.expectedRevision).toBe(2);
	});

	it('returns false when a render failure arrives after the transition was cancelled', async () => {
		const request = transition2D();
		await flushMicrotasks();
		await request;
		expect(cancelPreviewTransition({ restore: false })).toBe(true);
		expect(failPreviewTransition(new Error('late render error'))).toBe(false);
		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toBe('');
	});

	it('ignores stale acknowledgements when a faster 2D/3D intent replaces the old one', async () => {
		const pending: Array<(result: unknown) => void> = [];
		postMock.mockImplementation(() => new Promise((resolve) => pending.push(resolve)));
		const older = transition2D();
		await flushMicrotasks();
		const olderOptions = postMock.mock.calls[0][2] as { signal: AbortSignal };

		const newer = requestPreviewTransition(
			'component',
			{ component: '3D' },
			'Switching to 3D volume',
			{ type: '3D' },
			{ timeoutMs: 10000 }
		);
		await flushMicrotasks();
		expect(olderOptions.signal.aborted).toBe(true);
		const newerId = get(previewTransition)?.id;

		pending[0]({
			ok: true,
			data: { requestId: postMock.mock.calls[0][1].requestId, configRevision: 2 }
		});
		await older;
		expect(get(previewTransition)?.id).toBe(newerId);

		pending[1]({
			ok: true,
			data: { requestId: postMock.mock.calls[1][1].requestId, configRevision: 3 }
		});
		await newer;
		const stale2DFrame = currentFrame({
			type: '2D',
			component: 'x',
			sequence: 11,
			configRevision: 2
		});
		expect(finishPreviewTransition(stale2DFrame as never)).toBe(false);
		expect(get(previewTransition)?.id).toBe(newerId);
		const newest3DFrame = currentFrame({ type: '3D', sequence: 12, configRevision: 3 });
		expect(finishPreviewTransition(newest3DFrame as never)).toBe(true);
		expect(get(previewTransition)).toBeNull();
	});

	it('allows a user retry after a failed request and never retries automatically', async () => {
		postMock.mockResolvedValueOnce({ ok: false, error: 'Unavailable', status: 503 });
		await transition2D();
		expect(postMock).toHaveBeenCalledOnce();
		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toBe('Unavailable');

		const retry = retryPreviewTransition();
		expect(retry).toBeDefined();
		await flushMicrotasks();
		await retry;
		expect(postMock).toHaveBeenCalledTimes(2);
		expect(postMock.mock.calls[0][1].requestId).not.toBe(postMock.mock.calls[1][1].requestId);
		expect(get(previewTransition)?.expectedRevision).toBe(2);
	});

	it('provides a resync action and cancels pending work without showing an error', async () => {
		const recovery = vi.fn();
		setPreviewRecoveryHandler(recovery);
		expect(requestPreviewResync()).toBe(true);
		expect(recovery).toHaveBeenCalledOnce();

		const request = transition2D();
		await flushMicrotasks();
		expect(cancelPreviewTransition({ restore: false })).toBe(true);
		await request;
		expect(get(previewTransition)).toBeNull();
		expect(get(previewTransitionError)).toBe('');
		expect(vi.getTimerCount()).toBe(0);
	});

	it('clears error and retry state when disposed after a failed transition', async () => {
		postMock.mockResolvedValueOnce({ ok: false, error: 'Unavailable', status: 503 });
		await transition2D();
		expect(get(previewTransitionError)).toBe('Unavailable');

		expect(cancelPreviewTransition({ restore: false })).toBe(false);
		expect(get(previewTransitionError)).toBe('');
		expect(retryPreviewTransition()).toBeUndefined();
	});
});
