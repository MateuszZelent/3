import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { previewTransition } from '$api/previewTransition';
import { post } from '$api/post';
import {
	changeRegion,
	cancelRegionUpdates,
	regionDraft,
	regionError,
	regionPending,
	waitForRegionUpdate
} from './preview3DRegion';

vi.mock('$api/post', () => ({ post: vi.fn() }));
vi.mock('$api/websocket', () => ({ setPreviewClientBudget: vi.fn() }));

const postMock = vi.mocked(post);

function appliedRegion(body: Record<string, unknown>, configRevision = 1) {
	return {
		ok: true as const,
		data: {
			...body,
			configRevision
		}
	};
}

describe('3D region synchronization', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		cancelRegionUpdates();
		previewTransition.set(null);
		regionError.set('');
		regionPending.set(false);
		postMock.mockReset();
	});
	afterEach(() => {
		cancelRegionUpdates();
		previewTransition.set(null);
		vi.useRealTimers();
	});

	it('flushes a pending debounced region before the view transition request', async () => {
		postMock.mockImplementation(async (_endpoint, body) =>
			appliedRegion(body as Record<string, unknown>)
		);
		changeRegion({ enabled: true, start: [2, 0, 0] });
		previewTransition.set({
			id: 1,
			requestId: 'transition',
			label: 'Switching',
			phase: 'backend',
			startedAt: Date.now(),
			after: 0,
			type: '2D'
		});

		await waitForRegionUpdate();

		expect(postMock).toHaveBeenCalledOnce();
		expect(postMock.mock.calls[0][0]).toBe('preview/region');
		expect(postMock.mock.calls[0][1]).toMatchObject({
			enabled: true,
			start: [2, 0, 0],
			requestId: expect.any(String)
		});
		expect(get(regionPending)).toBe(false);
		expect(get(regionError)).toBe('');
	});

	it('rejects a transition flush when region update failed and exposes retryable error', async () => {
		postMock.mockResolvedValue({ ok: false, error: 'proxy unavailable', status: 502 });
		changeRegion({ enabled: true });

		await expect(waitForRegionUpdate()).rejects.toThrow('proxy unavailable');
		expect(get(regionPending)).toBe(false);
		expect(get(regionError)).toBe('proxy unavailable');
	});

	it('ignores new region mutations while the shared preview transition owns the controls', async () => {
		previewTransition.set({
			id: 1,
			requestId: 'transition',
			label: 'Switching',
			phase: 'backend',
			startedAt: Date.now(),
			after: 0,
			type: '2D'
		});
		const before = get(regionDraft);

		expect(changeRegion({ enabled: !before.enabled })).toBe(false);
		await waitForRegionUpdate();

		expect(postMock).not.toHaveBeenCalled();
		expect(get(regionDraft)).toBe(before);
	});

	it('cancels debounce and ignores an old acknowledgement after reopening controls', async () => {
		changeRegion({ enabled: true, start: [3, 0, 0] });
		cancelRegionUpdates();
		await vi.advanceTimersByTimeAsync(120);
		expect(postMock).not.toHaveBeenCalled();

		let resolveOld!: (value: ReturnType<typeof appliedRegion>) => void;
		postMock.mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					resolveOld = resolve;
				})
		);
		changeRegion({ enabled: true, start: [4, 0, 0] });
		const oldFlush = waitForRegionUpdate();
		const oldBody = postMock.mock.calls[0][1] as Record<string, unknown>;
		const signal = postMock.mock.calls[0][2]?.signal;
		cancelRegionUpdates();
		expect(signal?.aborted).toBe(true);

		postMock.mockImplementation(async (_endpoint, body) =>
			appliedRegion(body as Record<string, unknown>, 3)
		);
		changeRegion({ enabled: true, start: [5, 0, 0] });
		await waitForRegionUpdate();
		resolveOld(appliedRegion(oldBody, 2));
		await expect(oldFlush).rejects.toThrow('controls were closed');
		expect(get(regionDraft).start).toEqual([5, 0, 0]);
		expect(get(regionPending)).toBe(false);
		expect(get(regionError)).toBe('');
	});
});
