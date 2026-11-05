import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { solverState } from '../incoming/solver';
import { postRun, postSteps, postBreak, solverCommandPending } from './solver';

const command = vi.hoisted(() => vi.fn());
vi.mock('$api/post', () => ({ post: command }));
vi.mock('$lib/alerts/alert', () => ({ setAlert: vi.fn() }));
beforeEach(() => {
	command.mockReset();
	solverCommandPending.set(false);
	solverState.update((state) => ({
		...state,
		operationId: 0,
		operationStatus: 'idle',
		operationError: ''
	}));
});
describe('accepted solver operations', () => {
	it('shows server acceptance immediately without waiting for the entire simulation', async () => {
		command.mockResolvedValue({ ok: true, data: { operationId: 7, status: 'pending' } });
		await postRun('1e-9');
		expect(command).toHaveBeenCalledWith(
			'solver/run',
			{ runtime: '1e-9' },
			{ parseResponse: true }
		);
		expect(get(solverState)).toMatchObject({ operationId: 7, operationStatus: 'pending' });
	});
	it('does not overwrite a completed WebSocket result with a delayed HTTP acknowledgement', async () => {
		command.mockImplementation(async () => {
			solverState.update((state) => ({ ...state, operationId: 8, operationStatus: 'completed' }));
			return { ok: true, data: { operationId: 8, status: 'pending' } };
		});
		await postRun('1e-9');
		expect(get(solverState).operationStatus).toBe('completed');
	});
	it('does not submit duplicate operations before the first acknowledgement', async () => {
		let finish: ((value: unknown) => void) | undefined;
		command.mockReturnValue(
			new Promise((resolve) => {
				finish = resolve;
			})
		);
		const first = postRun('1e-9');
		expect(get(solverCommandPending)).toBe(true);
		expect((await postSteps('10')).ok).toBe(false);
		expect(command).toHaveBeenCalledOnce();
		finish!({ ok: true, data: { operationId: 9, status: 'pending' } });
		await first;
		expect(get(solverCommandPending)).toBe(false);
	});
	it('keeps Break available while an accepted operation is pending', async () => {
		solverState.update((state) => ({ ...state, operationId: 10, operationStatus: 'pending' }));
		command.mockResolvedValue({ ok: true });
		expect((await postBreak()).ok).toBe(true);
		expect(command).toHaveBeenCalledWith('solver/break', {});
	});
});
