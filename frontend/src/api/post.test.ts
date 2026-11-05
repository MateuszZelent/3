import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { post } from './post';

const alert = vi.hoisted(() => vi.fn());
vi.mock('$lib/alerts/alert', () => ({ setAlert: alert }));

describe('HTTP commands', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		alert.mockClear();
	});
	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});
	it('reports an HTTP failure to its caller and preserves the backend message', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(new Response('{"error":"Unavailable"}', { status: 503 }))
		);
		expect(await post('preview/component', { component: 'x' })).toEqual({
			ok: false,
			error: 'Unavailable',
			status: 503
		});
		expect(alert).toHaveBeenCalledWith('Unavailable');
	});
	it('returns network errors without leaving an unhandled rejection', async () => {
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Network failed')));
		expect(await post('solver/run', { runtime: '1e-9' })).toEqual({
			ok: false,
			error: 'Network failed'
		});
		expect(alert).toHaveBeenCalledOnce();
	});
	it('aborts a pending request at its deadline and never retries the command', async () => {
		const fetch = vi.fn(
			(_url, options: RequestInit) =>
				new Promise((_resolve, reject) => {
					options.signal!.addEventListener('abort', () => reject(options.signal!.reason));
				})
		);
		vi.stubGlobal('fetch', fetch);
		const pending = post('solver/run', { runtime: '1e-9' }, { timeoutMs: 100 });
		await vi.advanceTimersByTimeAsync(100);
		expect(await pending).toEqual({ ok: false, error: 'Request timed out' });
		expect(fetch).toHaveBeenCalledOnce();
	});
	it('supports caller cancellation without a spurious alert', async () => {
		const controller = new AbortController();
		controller.abort();
		const fetch = vi.fn();
		vi.stubGlobal('fetch', fetch);
		expect(await post('preview/component', {}, { signal: controller.signal })).toEqual({
			ok: false,
			error: 'Request cancelled'
		});
		expect(fetch).not.toHaveBeenCalled();
		expect(alert).not.toHaveBeenCalled();
	});
	it('cleans up its deadline after success', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
		expect(await post('solver/break', {})).toEqual({ ok: true });
		expect(vi.getTimerCount()).toBe(0);
	});
});
