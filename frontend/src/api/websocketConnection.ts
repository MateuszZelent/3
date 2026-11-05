type Handlers = {
	open: (socket: WebSocket) => void;
	close: () => void;
	message: (data: ArrayBuffer) => void;
	error?: (error: unknown) => void;
};

/** One socket and one reconnect timer, both owned by the returned disposer. */
export function connectWebSocket(url: string, handlers: Handlers): () => void {
	let socket: WebSocket | null = null;
	let retry: ReturnType<typeof setTimeout> | undefined;
	let stopped = false;
	let failures = 0;
	const online = () => typeof navigator === 'undefined' || navigator.onLine !== false;
	function reconnect() {
		if (stopped || retry !== undefined || !online()) return;
		const delay = Math.min(1000 * 2 ** Math.min(failures++, 5), 30000);
		retry = setTimeout(() => {
			retry = undefined;
			connect();
		}, delay);
	}
	function connect() {
		if (stopped || socket || !online()) return;
		let current: WebSocket;
		try {
			current = new WebSocket(url);
		} catch (error) {
			handlers.error?.(error);
			reconnect();
			return;
		}
		socket = current;
		let openedAt: number | undefined;
		current.binaryType = 'arraybuffer';
		current.onopen = () => {
			if (stopped || socket !== current) return;
			openedAt = Date.now();
			handlers.open(current);
		};
		current.onmessage = (event) => {
			if (!stopped && socket === current) handlers.message(event.data as ArrayBuffer);
		};
		current.onclose = () => {
			if (stopped || socket !== current) return;
			socket = null;
			if (openedAt !== undefined && Date.now() - openedAt >= 10000) failures = 0;
			handlers.close();
			reconnect();
		};
		current.onerror = (error) => {
			if (stopped || socket !== current) return;
			handlers.error?.(error);
			current.close();
		};
	}
	const resume = () => {
		if (stopped || !online() || socket) return;
		clearTimeout(retry);
		retry = undefined;
		connect();
	};
	window.addEventListener('online', resume);
	connect();
	return () => {
		if (stopped) return;
		stopped = true;
		clearTimeout(retry);
		retry = undefined;
		window.removeEventListener('online', resume);
		if (socket) {
			const current = socket;
			socket = null;
			current.onopen = current.onclose = current.onerror = current.onmessage = null;
			current.close();
		}
	};
}
