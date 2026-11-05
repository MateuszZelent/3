import { describe, expect, it } from 'vitest';
import { PreviewRenderGate, type PreviewFrameIdentity } from './previewRenderGate';

function frame(sequence: number, value = 1): PreviewFrameIdentity {
	return {
		sequence,
		topologyRevision: sequence,
		values: new Float32Array([value, 0, 0]),
		positions: new Int32Array([0, 0, 0])
	};
}

describe('preview render ACK gate', () => {
	it('only completes after a draw for the current frame', async () => {
		const gate = new PreviewRenderGate();
		const current = frame(1);
		const completion = gate.waitFor(current);
		gate.complete(current);
		await expect(completion).resolves.toBe(true);
	});

	it('transfers pending completion across same-frame camera or uniform updates', async () => {
		const gate = new PreviewRenderGate();
		const current = frame(2);
		const completion = gate.waitFor(current);
		gate.setCurrent(current);
		gate.complete(current);
		await expect(completion).resolves.toBe(true);
	});

	it('marks replaced frames and disposed work as not rendered', async () => {
		const gate = new PreviewRenderGate();
		const oldCompletion = gate.waitFor(frame(3));
		gate.setCurrent(frame(4));
		await expect(oldCompletion).resolves.toBe(false);
		const currentCompletion = gate.waitFor(frame(5));
		gate.dispose();
		await expect(currentCompletion).resolves.toBe(false);
	});
});
