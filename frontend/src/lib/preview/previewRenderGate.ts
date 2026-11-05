export interface PreviewFrameIdentity {
	sequence?: number;
	topologyRevision: number;
	values: Float32Array;
	positions: Int32Array;
}

type Waiter = {
	identity: PreviewFrameIdentity;
	resolve: (rendered: boolean) => void;
};

export function samePreviewFrame(a: PreviewFrameIdentity, b: PreviewFrameIdentity) {
	return (
		a.sequence === b.sequence &&
		a.topologyRevision === b.topologyRevision &&
		a.values === b.values &&
		a.positions === b.positions
	);
}

/** Settles network-frame waiters only after a matching frame has been drawn. */
export class PreviewRenderGate {
	private current: PreviewFrameIdentity | null = null;
	private readonly waiters = new Set<Waiter>();

	waitFor(identity: PreviewFrameIdentity): Promise<boolean> {
		this.setCurrent(identity);
		return new Promise((resolve) => this.waiters.add({ identity, resolve }));
	}

	setCurrent(identity: PreviewFrameIdentity) {
		this.current = identity;
		for (const waiter of [...this.waiters]) {
			if (!samePreviewFrame(waiter.identity, identity)) this.finish(waiter, false);
		}
	}

	complete(identity: PreviewFrameIdentity) {
		if (!this.current || !samePreviewFrame(this.current, identity)) return;
		for (const waiter of [...this.waiters]) {
			if (samePreviewFrame(waiter.identity, identity)) this.finish(waiter, true);
		}
	}

	fail(identity: PreviewFrameIdentity) {
		if (!this.current || !samePreviewFrame(this.current, identity)) return;
		for (const waiter of [...this.waiters]) {
			if (samePreviewFrame(waiter.identity, identity)) this.finish(waiter, false);
		}
	}

	dispose() {
		this.current = null;
		for (const waiter of [...this.waiters]) this.finish(waiter, false);
	}

	private finish(waiter: Waiter, rendered: boolean) {
		this.waiters.delete(waiter);
		waiter.resolve(rendered);
	}
}
