import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	postFullResolution,
	postPlaneResolution,
	postXChosenSize,
	postSection,
	waitForPreviewMutations,
	cancelPreviewMutations
} from './preview';
import { previewTransition } from '../previewTransition';

const postMock = vi.hoisted(() => vi.fn());
vi.mock('$api/post', () => ({ post: postMock }));

function deferred<T>() {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((complete) => (resolve = complete));
	return { promise, resolve };
}

async function flushMicrotasks() {
	await Promise.resolve();
	await Promise.resolve();
	await Promise.resolve();
}

function ackFor(callIndex: number, configRevision: number) {
	return {
		ok: true,
		data: { requestId: postMock.mock.calls[callIndex][1].requestId, configRevision }
	};
}

describe('preview config mutation queue', () => {
	beforeEach(() => {
		cancelPreviewMutations();
		previewTransition.set(null);
		postMock.mockReset();
	});
	afterEach(() => {
		cancelPreviewMutations();
		previewTransition.set(null);
		vi.clearAllTimers();
	});

	it('serializes size changes and sends only the latest queued value for the same axis', async () => {
		const first = deferred<unknown>();
		const latest = deferred<unknown>();
		postMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(latest.promise);

		const initial = postXChosenSize(2);
		await flushMicrotasks();
		const superseded = postXChosenSize(4);
		const final = postXChosenSize(8);
		expect(postMock).toHaveBeenCalledOnce();

		first.resolve(ackFor(0, 1));
		await initial;
		await flushMicrotasks();
		expect(postMock).toHaveBeenCalledTimes(2);
		expect(postMock.mock.calls[1][0]).toBe('preview/XChosenSize');
		expect(postMock.mock.calls[1][1]).toMatchObject({
			xChosenSize: 8,
			requestId: expect.any(String)
		});

		latest.resolve(ackFor(1, 2));
		await Promise.all([superseded, final, waitForPreviewMutations()]);
		expect(postMock).toHaveBeenCalledTimes(2);
	});

	it('merges queued plane-resolution axes so one latest update cannot discard the other', async () => {
		const first = deferred<unknown>();
		const combined = deferred<unknown>();
		postMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(combined.promise);

		const active = postXChosenSize(2);
		await flushMicrotasks();
		const u = postPlaneResolution({ uSize: 4 });
		const v = postPlaneResolution({ vSize: 6 });
		first.resolve(ackFor(0, 1));
		await active;
		await flushMicrotasks();
		expect(postMock.mock.calls[1][1]).toMatchObject({ uSize: 4, vSize: 6 });
		combined.resolve(ackFor(1, 2));
		await Promise.all([u, v, waitForPreviewMutations()]);
		expect(postMock).toHaveBeenCalledTimes(2);
	});

	it('does not send a queued size update while a transition owns the preview', async () => {
		previewTransition.set({
			id: 3,
			requestId: 'transition',
			label: 'Switching',
			phase: 'backend',
			startedAt: Date.now(),
			after: 1,
			type: '2D'
		});
		const results = await Promise.all([
			postXChosenSize(4),
			postPlaneResolution({ uSize: 4 }),
			postSection({ plane: 'yz' })
		]);
		expect(postMock).not.toHaveBeenCalled();
		expect(results.every((result) => !result.ok)).toBe(true);
	});

	it('makes full resolution supersede older queued axis sizes', async () => {
		const activePost = deferred<unknown>();
		const full = deferred<unknown>();
		postMock.mockReturnValueOnce(activePost.promise).mockReturnValueOnce(full.promise);
		const active = postXChosenSize(2);
		await flushMicrotasks();
		const oldY = postPlaneResolution({ vSize: 6 });
		const fullResolution = postFullResolution();
		activePost.resolve(ackFor(0, 1));
		await active;
		await flushMicrotasks();
		expect(postMock).toHaveBeenCalledTimes(2);
		expect(postMock.mock.calls[1][0]).toBe('preview/fullResolution');
		full.resolve(ackFor(1, 2));
		await Promise.all([oldY, fullResolution, waitForPreviewMutations()]);
	});

	it('aborts teardown work, drops unsent changes and ignores a late acknowledgement', async () => {
		const old = deferred<unknown>();
		const fresh = deferred<unknown>();
		postMock.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
		const first = postXChosenSize(2);
		const unsent = postXChosenSize(4);
		const signal = postMock.mock.calls[0][2].signal as AbortSignal;
		cancelPreviewMutations();
		expect(signal.aborted).toBe(true);
		expect((await first).ok).toBe(false);
		expect((await unsent).ok).toBe(false);
		const next = postXChosenSize(8);
		old.resolve(ackFor(0, 1));
		await flushMicrotasks();
		expect(postMock).toHaveBeenCalledTimes(2);
		fresh.resolve(ackFor(1, 2));
		expect((await next).ok).toBe(true);
		await waitForPreviewMutations();
	});
});
