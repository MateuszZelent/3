import { describe, expect, it, vi } from 'vitest';
import { createVisibleFrameTask } from './visible-frame-task';

function fakeFrames() {
	let nextId = 0;
	const pending = new Map<number, FrameRequestCallback>();
	return {
		request: vi.fn((callback: FrameRequestCallback) => {
			const id = ++nextId;
			pending.set(id, callback);
			return id;
		}),
		cancel: vi.fn((id: number) => pending.delete(id)),
		runAll() {
			const callbacks = [...pending.values()];
			pending.clear();
			for (const callback of callbacks) callback(0);
		},
		get size() {
			return pending.size;
		}
	};
}

describe('visible frame task', () => {
	it('does not schedule hidden work and coalesces visible updates to one frame', () => {
		const frames = fakeFrames();
		const run = vi.fn();
		const task = createVisibleFrameTask(run, frames.request, frames.cancel);

		task.schedule(false);
		expect(frames.request).not.toHaveBeenCalled();

		task.schedule(true);
		task.schedule(true);
		task.schedule(true);
		expect(frames.size).toBe(1);
		expect(frames.request).toHaveBeenCalledTimes(1);

		frames.runAll();
		expect(run).toHaveBeenCalledTimes(1);
	});

	it('cancels pending work when hidden and on disposal', () => {
		const frames = fakeFrames();
		const run = vi.fn();
		const task = createVisibleFrameTask(run, frames.request, frames.cancel);

		task.schedule(true);
		task.schedule(false);
		expect(frames.size).toBe(0);
		frames.runAll();
		expect(run).not.toHaveBeenCalled();

		task.schedule(true);
		task.dispose();
		expect(frames.size).toBe(0);
		frames.runAll();
		expect(run).not.toHaveBeenCalled();
	});
});
