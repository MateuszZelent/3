import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { consoleState } from '../incoming/console';
import { solverState } from '../incoming/solver';
import { postCommand, consoleCommandPending } from './console';

const command = vi.hoisted(() => vi.fn());
vi.mock('$api/post', () => ({ post: command }));
vi.mock('$lib/alerts/alert', () => ({ setAlert: vi.fn() }));
beforeEach(() => {
	command.mockReset();
	consoleCommandPending.set(false);
	consoleState.set({ hist: '', operationId: 0, operationStatus: 'idle' });
	solverState.update((state) => ({ ...state, operationId: 10, operationStatus: 'running' }));
});
describe('console commands during simulation', () => {
	it('accepts an independent console command while the solver is running', async () => {
		command.mockResolvedValue({ ok: true, data: { operationId: 1, status: 'pending' } });
		expect((await postCommand('print(t)')).ok).toBe(true);
		expect(command).toHaveBeenCalledWith(
			'console/command',
			{ command: 'print(t)' },
			{ parseResponse: true }
		);
		expect(get(consoleState)).toMatchObject({ operationId: 1, operationStatus: 'pending' });
		expect(get(solverState)).toMatchObject({ operationId: 10, operationStatus: 'running' });
	});
	it('retains a completed console result which arrives before HTTP acceptance', async () => {
		command.mockImplementation(async () => {
			consoleState.set({ hist: 'done', operationId: 2, operationStatus: 'completed' });
			return { ok: true, data: { operationId: 2, status: 'pending' } };
		});
		await postCommand('print(t)');
		expect(get(consoleState).operationStatus).toBe('completed');
	});
	it('blocks only a second console operation, not the solver lane', async () => {
		consoleState.set({ hist: '', operationId: 3, operationStatus: 'pending' });
		expect((await postCommand('print(t)')).ok).toBe(false);
		expect(command).not.toHaveBeenCalled();
	});
});
