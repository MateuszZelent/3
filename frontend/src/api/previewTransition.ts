import { get, writable } from 'svelte/store';
import type { Preview } from './incoming/preview';
import { previewState } from './incoming/preview';
export const previewTransition = writable<{
	id: number;
	label: string;
	phase: 'backend' | 'rendering';
	startedAt: number;
	after: number;
	type?: string;
	quantity?: string;
	component?: string;
} | null>(null);
export const previewTransitionError = writable('');
let nextId = 0;
export async function requestPreviewTransition(
	endpoint: string,
	data: Record<string, unknown>,
	label: string,
	target: { type?: string; quantity?: string; component?: string }
) {
	const id = ++nextId;
	previewTransitionError.set('');
	previewTransition.set({
		id,
		label,
		phase: 'backend',
		startedAt: Date.now(),
		after: get(previewState).sequence || 0,
		...target
	});
	// Yield a paint before the request and any resulting decode/render work.
	await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
	try {
		const response = await fetch(`./api/preview/${endpoint}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(data)
		});
		if (!response.ok) {
			const body = await response.json();
			throw new Error(body.error || 'Preview update failed');
		}
	} catch (error) {
		if (get(previewTransition)?.id === id) {
			previewTransitionError.set(String(error));
			previewTransition.set(null);
		}
	}
}
function matches(state: Preview) {
	const t = get(previewTransition);
	return (
		t &&
		(state.sequence || 0) > t.after &&
		(!t.type || state.type === t.type) &&
		(!t.quantity || state.quantity === t.quantity) &&
		(!t.component || state.component === t.component)
	);
}
export function markPreviewRendering(state: Preview) {
	if (matches(state)) previewTransition.update((t) => (t ? { ...t, phase: 'rendering' } : t));
}
export function finishPreviewTransition(state: Preview) {
	if (matches(state)) previewTransition.set(null);
}
