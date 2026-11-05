import { derived, get, writable } from 'svelte/store';
import { post, type PostResult } from '$api/post';
import { solverState } from '$api/incoming/solver';
import { setAlert } from '$lib/alerts/alert';
import { postAcceptedCommand, isEngineOperationActive } from '../acceptedCommand';

export const solverCommandPending = writable(false);
export const solverOperationBusy = derived(
	[solverState, solverCommandPending],
	([state, pending]) => pending || isEngineOperationActive(state)
);

async function submitSolverCommand(endpoint: string, data: object): Promise<PostResult> {
	if (get(solverOperationBusy)) {
		const error = 'A solver operation is already pending. Use Break to stop it.';
		setAlert(error);
		return { ok: false, error };
	}
	solverCommandPending.set(true);
	try {
		const result = await postAcceptedCommand(endpoint, data);
		if (!result.ok) return result;
		const operationId = result.data.operationId;
		solverState.update((state) =>
			(state.operationId ?? 0) >= operationId
				? state
				: { ...state, operationId, operationStatus: 'pending', operationError: '' }
		);
		return result;
	} finally {
		solverCommandPending.set(false);
	}
}

export function postSolverType(type: string) {
	return post('solver/type', { type });
}

export function postRun(runtime: string) {
	return submitSolverCommand('solver/run', { runtime });
}

export function postSteps(steps: string) {
	return submitSolverCommand('solver/steps', { steps });
}

export function postRelax() {
	return submitSolverCommand('solver/relax', {});
}

export function postMinimize() {
	return submitSolverCommand('solver/minimize', {});
}

export function postBreak() {
	return post('solver/break', {});
}

export function postFixdt(fixdt = get(solverState).fixdt) {
	return post('solver/fixdt', { fixdt });
}

export function postMindt(mindt = get(solverState).mindt) {
	return post('solver/mindt', { mindt });
}

export function postMaxdt(maxdt = get(solverState).maxdt) {
	return post('solver/maxdt', { maxdt });
}

export function postMaxerr(maxerr = get(solverState).maxerr) {
	return post('solver/maxerr', { maxerr });
}
