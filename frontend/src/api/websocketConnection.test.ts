import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { connectWebSocket } from './websocketConnection';

class FakeSocket {
	static instances: FakeSocket[] = [];
	binaryType = '';
	onopen: (() => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: ((error: unknown) => void) | null = null;
	onmessage: ((event: { data: ArrayBuffer }) => void) | null = null;
	close = vi.fn(() => this.onclose?.());
	constructor(readonly url: string) {
		FakeSocket.instances.push(this);
	}
}
beforeEach(() => {
	vi.useFakeTimers();
	FakeSocket.instances = [];
	vi.stubGlobal('WebSocket', FakeSocket);
	vi.stubGlobal('window', new EventTarget());
	vi.stubGlobal('navigator', { onLine: true });
});
afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe('websocket lifecycle', () => {
	it('backs off repeated failures and cancels the pending reconnect on disposal', () => {
		const dispose = connectWebSocket('./ws', { open: vi.fn(), close: vi.fn(), message: vi.fn() });
		FakeSocket.instances[0].onclose!();
		vi.advanceTimersByTime(999);
		expect(FakeSocket.instances).toHaveLength(1);
		vi.advanceTimersByTime(1);
		expect(FakeSocket.instances).toHaveLength(2);
		FakeSocket.instances[1].onclose!();
		vi.advanceTimersByTime(1999);
		expect(FakeSocket.instances).toHaveLength(2);
		dispose();
		vi.advanceTimersByTime(60000);
		expect(FakeSocket.instances).toHaveLength(2);
		expect(vi.getTimerCount()).toBe(0);
	});
	it('ignores stale callbacks after teardown and closes exactly once', () => {
		const message = vi.fn();
		const close = vi.fn();
		const dispose = connectWebSocket('./ws', { open: vi.fn(), close, message });
		const socket = FakeSocket.instances[0];
		const oldMessage = socket.onmessage!;
		dispose();
		dispose();
		oldMessage({ data: new ArrayBuffer(0) });
		expect(message).not.toHaveBeenCalled();
		expect(close).not.toHaveBeenCalled();
		expect(socket.close).toHaveBeenCalledOnce();
	});
	it('also backs off connections which open and immediately drop', () => {
		const dispose = connectWebSocket('./ws', { open: vi.fn(), close: vi.fn(), message: vi.fn() });
		FakeSocket.instances[0].onopen!();
		FakeSocket.instances[0].onclose!();
		vi.advanceTimersByTime(1000);
		FakeSocket.instances[1].onopen!();
		FakeSocket.instances[1].onclose!();
		vi.advanceTimersByTime(1000);
		expect(FakeSocket.instances).toHaveLength(2);
		vi.advanceTimersByTime(1000);
		expect(FakeSocket.instances).toHaveLength(3);
		dispose();
	});
	it('waits while offline and resumes immediately on the online event', () => {
		vi.stubGlobal('navigator', { onLine: false });
		const dispose = connectWebSocket('./ws', { open: vi.fn(), close: vi.fn(), message: vi.fn() });
		vi.advanceTimersByTime(60000);
		expect(FakeSocket.instances).toHaveLength(0);
		vi.stubGlobal('navigator', { onLine: true });
		window.dispatchEvent(new Event('online'));
		expect(FakeSocket.instances).toHaveLength(1);
		dispose();
	});
});
