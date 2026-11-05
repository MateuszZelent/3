export type RequestFrame = (callback: FrameRequestCallback) => number;
export type CancelFrame = (id: number) => void;

export function createVisibleFrameTask(
	work: () => void,
	requestFrame: RequestFrame = (callback) => requestAnimationFrame(callback),
	cancelFrame: CancelFrame = (id) => cancelAnimationFrame(id)
) {
	let frame: number | undefined;
	let visible = false;

	function cancel() {
		visible = false;
		if (frame !== undefined) {
			cancelFrame(frame);
			frame = undefined;
		}
	}

	return {
		schedule(isVisible: boolean) {
			visible = isVisible;
			if (!visible) {
				cancel();
				return;
			}
			if (frame !== undefined) return;
			frame = requestFrame(() => {
				frame = undefined;
				if (visible) work();
			});
		},
		cancel,
		dispose: cancel
	};
}
