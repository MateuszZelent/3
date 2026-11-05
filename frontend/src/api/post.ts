import { setAlert } from '$lib/alerts/alert';

export type PostResult =
	{ ok: true; data?: unknown } | { ok: false; error: string; status?: number };
export type PostOptions = {
	signal?: AbortSignal;
	timeoutMs?: number;
	alert?: boolean;
	parseResponse?: boolean;
};
export const DEFAULT_REQUEST_TIMEOUT_MS = 15000;

/** Send a command once. A timeout does not imply that the server rolled it back. */
export async function post(
	endpoint: string,
	data: object,
	options: PostOptions = {}
): Promise<PostResult> {
	if (options.signal?.aborted) return { ok: false, error: 'Request cancelled' };
	const controller = new AbortController();
	const cancel = () => controller.abort();
	options.signal?.addEventListener('abort', cancel, { once: true });
	let timedOut = false;
	const deadline = setTimeout(() => {
		timedOut = true;
		controller.abort();
	}, options.timeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS);
	function failure(error: string, status?: number): PostResult {
		if (options.alert !== false && !options.signal?.aborted) setAlert(error);
		return status === undefined ? { ok: false, error } : { ok: false, error, status };
	}
	try {
		const response = await fetch(`./api/${endpoint}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(data),
			signal: controller.signal
		});
		if (!response.ok) {
			let message = `Request failed (${response.status})`;
			try {
				const body: unknown = await response.json();
				if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string')
					message = body.error;
			} catch {
				// Proxies can return HTML or an empty response instead of API JSON.
			}
			return failure(message, response.status);
		}
		if (options.parseResponse && response.status !== 204) {
			const data: unknown = await response.json();
			return { ok: true, data };
		}
		return { ok: true };
	} catch (error) {
		return failure(
			timedOut
				? 'Request timed out'
				: options.signal?.aborted
					? 'Request cancelled'
					: error instanceof Error
						? error.message
						: 'Request failed'
		);
	} finally {
		clearTimeout(deadline);
		options.signal?.removeEventListener('abort', cancel);
	}
}
