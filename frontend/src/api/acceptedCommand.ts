import { post, type PostResult } from './post';
import { setAlert } from '$lib/alerts/alert';

export type EngineOperationStatus =
	'idle' | 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
export type EngineOperation = {
	operationId?: number;
	operationStatus?: EngineOperationStatus;
	operationError?: string;
};
type CommandAck = { operationId: number; status: 'pending' };
type AcceptedCommandResult = { ok: true; data: CommandAck } | Extract<PostResult, { ok: false }>;

export function isEngineOperationActive(state: EngineOperation) {
	return state.operationStatus === 'pending' || state.operationStatus === 'running';
}

/** Acknowledgement is bounded in time; the engine operation may run much longer. */
export async function postAcceptedCommand(
	endpoint: string,
	data: object
): Promise<AcceptedCommandResult> {
	const result = await post(endpoint, data, { parseResponse: true });
	if (!result.ok) return result;
	const ack = result.data;
	if (
		!ack ||
		typeof ack !== 'object' ||
		!('operationId' in ack) ||
		!Number.isSafeInteger(ack.operationId) ||
		Number(ack.operationId) <= 0 ||
		!('status' in ack) ||
		ack.status !== 'pending'
	) {
		const error = 'Server did not acknowledge the engine command';
		setAlert(error);
		return { ok: false, error };
	}
	return { ok: true, data: { operationId: Number(ack.operationId), status: 'pending' } };
}
