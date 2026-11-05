import { derived, get, writable } from 'svelte/store';
import type { Preview } from './incoming/preview';
import { previewState } from './incoming/preview';
import { post } from './post';

export type PreviewTransitionTarget = {
	type?: string;
	quantity?: string;
	component?: string;
};
export type PreviewTransitionOptions = {
	timeoutMs?: number;
	beforeRequest?: () => void | Promise<void>;
};

type PreviewConfigAck = { requestId: string; configRevision: number };
type TransitionRequest = {
	endpoint: string;
	data: Record<string, unknown>;
	label: string;
	target: PreviewTransitionTarget;
	timeoutMs: number;
	beforeRequest?: () => void | Promise<void>;
};
type ActiveTransition = {
	id: number;
	requestId: string;
	controller: AbortController;
	timer: ReturnType<typeof setTimeout>;
	request: TransitionRequest;
	after: number;
	beforeRevision: number;
	expectedRevision?: number;
};
type LateRecoveryExpectation = {
	after: number;
	beforeRevision: number;
	expectedRevision?: number;
	target: PreviewTransitionTarget;
};

export const PREVIEW_TRANSITION_TIMEOUT_MS = 10000;
export const previewTransition = writable<{
	id: number;
	requestId: string;
	label: string;
	phase: 'backend' | 'rendering';
	startedAt: number;
	after: number;
	expectedRevision?: number;
	type?: string;
	quantity?: string;
	component?: string;
} | null>(null);
export const previewTransitionError = writable('');
export const previewControlsBusy = derived(previewTransition, Boolean);

let nextId = 0;
const clientId = Math.random().toString(36).slice(2);
let active: ActiveTransition | null = null;
let lastRenderedPreview: Preview | null = null;
let lastFailedRequest: TransitionRequest | null = null;
let lateRecovery: LateRecoveryExpectation | null = null;
let recoveryHandler: (() => void) | null = null;

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function isConfigAck(data: unknown, requestId: string): data is PreviewConfigAck {
	if (!data || typeof data !== 'object') return false;
	const ack = data as { requestId?: unknown; configRevision?: unknown };
	return (
		ack.requestId === requestId &&
		typeof ack.configRevision === 'number' &&
		Number.isSafeInteger(ack.configRevision) &&
		ack.configRevision >= 0
	);
}

function matchesTarget(state: Preview, target: PreviewTransitionTarget): boolean {
	return (
		(!target.type || state.type === target.type) &&
		(!target.quantity || state.quantity === target.quantity) &&
		(!target.component || state.component === target.component)
	);
}

// The server may advance configRevision past the acknowledged one before the
// capture is published (e.g. a finished console operation requests a refresh),
// and then never sends the acknowledged revision. Accept any later frame that
// still shows the requested target.
function matchesActiveFrame(state: Preview, transition: ActiveTransition): boolean {
	return (
		transition.expectedRevision !== undefined &&
		(state.configRevision ?? -1) >= transition.expectedRevision &&
		(state.sequence || 0) > transition.after &&
		matchesTarget(state, transition.request.target)
	);
}

function clearActive(transition: ActiveTransition) {
	if (active?.id !== transition.id) return false;
	clearTimeout(transition.timer);
	active = null;
	previewTransition.set(null);
	return true;
}

function completeIfAlreadyRendered(transition: ActiveTransition) {
	if (!lastRenderedPreview || !matchesActiveFrame(lastRenderedPreview, transition)) return false;
	if (!clearActive(transition)) return false;
	lastFailedRequest = null;
	lateRecovery = null;
	previewTransitionError.set('');
	return true;
}

function matchesLateRecoveryFrame(state: Preview, expectation: LateRecoveryExpectation) {
	const sequence = state.sequence ?? 0;
	const revision = state.configRevision;
	const revisionMatches =
		typeof revision === 'number' &&
		Number.isSafeInteger(revision) &&
		(expectation.expectedRevision !== undefined
			? revision >= expectation.expectedRevision
			: revision > expectation.beforeRevision);
	return (
		sequence > expectation.after && revisionMatches && matchesTarget(state, expectation.target)
	);
}

function completeLateRecovery(state: Preview) {
	const expectation = lateRecovery;
	if (!expectation || !matchesLateRecoveryFrame(state, expectation)) return false;
	lateRecovery = null;
	lastFailedRequest = null;
	previewTransitionError.set('');
	return true;
}

function failActiveTransition(id: number, error: unknown): boolean {
	const transition = active;
	if (!transition || transition.id !== id) return false;

	clearTimeout(transition.timer);
	active = null;
	transition.controller.abort();
	if (lastRenderedPreview) previewState.set(lastRenderedPreview);
	lastFailedRequest = transition.request;
	lateRecovery = {
		after: transition.after,
		beforeRevision: transition.beforeRevision,
		expectedRevision: transition.expectedRevision,
		target: { ...transition.request.target }
	};
	previewTransitionError.set(errorMessage(error));
	previewTransition.set(null);

	try {
		recoveryHandler?.();
	} catch (recoveryError) {
		previewTransitionError.set(
			`${errorMessage(error)} Resync failed: ${errorMessage(recoveryError)}`
		);
	}
	return true;
}

export async function requestPreviewTransition(
	endpoint: string,
	data: Record<string, unknown>,
	label: string,
	target: PreviewTransitionTarget,
	options: PreviewTransitionOptions = {}
): Promise<void> {
	cancelPreviewTransition({ restore: false });
	const id = ++nextId;
	const timeoutMs = Math.max(1, options.timeoutMs ?? PREVIEW_TRANSITION_TIMEOUT_MS);
	const request: TransitionRequest = {
		endpoint,
		data: { ...data },
		label,
		target,
		timeoutMs,
		beforeRequest: options.beforeRequest
	};
	const requestId = `preview-${clientId}-${id}`;
	const controller = new AbortController();
	const currentPreview = get(previewState);
	const beforeRevision = Number.isSafeInteger(currentPreview.configRevision)
		? (currentPreview.configRevision as number)
		: 0;
	const transition: ActiveTransition = {
		id,
		requestId,
		controller,
		timer: setTimeout(
			() => failActiveTransition(id, new Error('Timed out waiting for the preview frame')),
			timeoutMs
		),
		request,
		after: currentPreview.sequence || 0,
		beforeRevision
	};
	active = transition;
	lastFailedRequest = null;
	previewTransitionError.set('');
	previewTransition.set({
		id,
		requestId,
		label,
		phase: 'backend',
		startedAt: Date.now(),
		after: transition.after,
		...target
	});

	try {
		await new Promise<void>((resolve) =>
			requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
		);
		if (active?.id !== id) return;
		await options.beforeRequest?.();
		if (active?.id !== id) return;

		const result = await post(
			`preview/${endpoint}`,
			{ ...data, requestId },
			{
				signal: controller.signal,
				timeoutMs,
				alert: false,
				parseResponse: true
			}
		);
		if (active?.id !== id) return;
		if (!result.ok) {
			failActiveTransition(id, result.error);
			return;
		}
		if (!isConfigAck(result.data, requestId)) {
			failActiveTransition(
				id,
				new Error('Preview update returned an invalid config acknowledgement')
			);
			return;
		}
		const expectedRevision = (result.data as PreviewConfigAck).configRevision;
		transition.expectedRevision = expectedRevision;
		previewTransition.update((current) =>
			current?.id === id ? { ...current, expectedRevision } : current
		);
		completeIfAlreadyRendered(transition);
	} catch (error) {
		failActiveTransition(id, error);
	}
}

export function markPreviewRendering(state: Preview): boolean {
	const transition = active;
	if (!transition || !matchesActiveFrame(state, transition)) return false;
	previewTransition.update((current) => {
		if (!current || current.id !== transition.id) return current;
		return { ...current, phase: 'rendering' };
	});
	return true;
}

export function finishPreviewTransition(state: Preview): boolean {
	lastRenderedPreview = state;
	const transition = active;
	if (!transition) return completeLateRecovery(state);
	if (!matchesActiveFrame(state, transition)) return false;
	if (!clearActive(transition)) return false;
	lastFailedRequest = null;
	lateRecovery = null;
	previewTransitionError.set('');
	return true;
}

export function failPreviewTransition(error: unknown): boolean {
	return active ? failActiveTransition(active.id, error) : false;
}

export function cancelPreviewTransition({ restore = false }: { restore?: boolean } = {}): boolean {
	const transition = active;
	if (!transition) {
		lastFailedRequest = null;
		lateRecovery = null;
		previewTransition.set(null);
		previewTransitionError.set('');
		return false;
	}
	clearTimeout(transition.timer);
	active = null;
	transition.controller.abort();
	if (restore && lastRenderedPreview) previewState.set(lastRenderedPreview);
	lastFailedRequest = null;
	lateRecovery = null;
	previewTransition.set(null);
	previewTransitionError.set('');
	return true;
}

export function setPreviewRecoveryHandler(handler: (() => void) | null) {
	recoveryHandler = handler;
}

export function requestPreviewResync(): boolean {
	if (!recoveryHandler) return false;
	try {
		recoveryHandler();
		return true;
	} catch (error) {
		previewTransitionError.set(`Resync failed: ${errorMessage(error)}`);
		return false;
	}
}

export function retryPreviewTransition(): Promise<void> | undefined {
	const request = lastFailedRequest;
	if (!request) return undefined;
	lastFailedRequest = null;
	return requestPreviewTransition(request.endpoint, request.data, request.label, request.target, {
		timeoutMs: request.timeoutMs,
		beforeRequest: request.beforeRequest
	});
}
