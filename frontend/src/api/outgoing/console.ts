import { derived, get, writable } from 'svelte/store';
import { consoleState } from '../incoming/console';
import { postAcceptedCommand, isEngineOperationActive } from '../acceptedCommand';
import type { PostResult } from '../post';
import { setAlert } from '$lib/alerts/alert';

export const consoleCommandPending = writable(false);
export const consoleOperationBusy = derived(
	[consoleState, consoleCommandPending],
	([state, pending]) => pending || isEngineOperationActive(state)
);

export async function postCommand(command: string): Promise<PostResult> {
	if (get(consoleOperationBusy)) {
		const error = 'A console command is already pending. Use Break to stop it.';
		setAlert(error);
		return { ok: false, error };
	}
	consoleCommandPending.set(true);
	try {
		const result = await postAcceptedCommand('console/command', { command });
		if (!result.ok) return result;
		const operationId = result.data.operationId;
		consoleState.update((state) =>
			(state.operationId ?? 0) >= operationId
				? state
				: { ...state, operationId, operationStatus: 'pending', operationError: '' }
		);
		return result;
	} finally {
		consoleCommandPending.set(false);
	}
}
