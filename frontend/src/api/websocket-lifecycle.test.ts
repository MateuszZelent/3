import { encode } from '@msgpack/msgpack';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writable } from 'svelte/store';

const hooks = vi.hoisted(() => ({
	finish: vi.fn(),
	fail: vi.fn(),
	cancel: vi.fn(),
	recovery: vi.fn(),
	render: vi.fn()
}));
vi.mock('./previewTransition', () => ({
	markPreviewRendering: vi.fn(),
	finishPreviewTransition: hooks.finish,
	failPreviewTransition: hooks.fail,
	cancelPreviewTransition: hooks.cancel,
	setPreviewRecoveryHandler: hooks.recovery
}));
vi.mock('$lib/preview/preview3D', () => ({
	preview3D: hooks.render,
	renderMode: writable('volume'),
	setRenderMode: vi.fn()
}));

class Socket {
	static OPEN = 1;
	static instances: Socket[] = [];
	readyState = 1;
	binaryType = '';
	onopen: (() => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: (() => void) | null = null;
	onmessage: ((event: { data: ArrayBuffer }) => void) | null = null;
	send = vi.fn();
	close = vi.fn();
	constructor(readonly url: string) {
		Socket.instances.push(this);
	}
}
let cleanup: (() => void) | undefined;
function previewFrame(topologyRevision = 7) {
	return encode({
		sequence: 1,
		configRevision: 3,
		type: '3D',
		nComp: 3,
		mesh: { Nx: 1, Ny: 1, Nz: 1, dx: 1, dy: 1, dz: 1 },
		vectorCount: 1,
		topologyRevision,
		vectorFieldValues: [{ x: 1, y: 0, z: 0 }],
		vectorFieldPositions: [{ x: 0, y: 0, z: 0 }]
	}).slice().buffer;
}
async function flushRender() {
	for (const callback of frames.values()) callback(0);
	frames.clear();
	await vi.dynamicImportSettled();
	await Promise.resolve();
	await Promise.resolve();
}
const frames = new Map<number, FrameRequestCallback>();
beforeEach(() => {
	vi.resetModules();
	vi.clearAllMocks();
	vi.useFakeTimers();
	Socket.instances = [];
	frames.clear();
	let id = 0;
	vi.stubGlobal('window', new EventTarget());
	vi.stubGlobal('document', Object.assign(new EventTarget(), { hidden: false }));
	vi.stubGlobal('navigator', { onLine: true });
	vi.stubGlobal('WebSocket', Socket);
	vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
		frames.set(++id, callback);
		return id;
	});
	vi.stubGlobal('cancelAnimationFrame', (frame: number) => frames.delete(frame));
});
afterEach(() => {
	cleanup?.();
	cleanup = undefined;
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe('application socket ownership', () => {
	it('returns credit for an already rendered resync replay without rendering it twice', async () => {
		hooks.render.mockResolvedValue(true);
		const api = await import('./websocket');
		cleanup = api.initializeWebSocket();
		Socket.instances.forEach((socket) => socket.onopen!());
		api.setPreviewSurface({
			isConnected: true,
			clientWidth: 640,
			clientHeight: 480
		} as HTMLElement);
		api.parsePreviewMsgpack(previewFrame());
		await flushRender();
		const socket = Socket.instances[1];
		expect(socket.send).toHaveBeenLastCalledWith(JSON.stringify({ ack: 1, revision: 7 }));
		socket.send.mockClear();
		api.setPreviewClientBudget(500000);
		api.parsePreviewMsgpack(previewFrame());
		expect(socket.send).toHaveBeenLastCalledWith(JSON.stringify({ ack: 1, revision: 7 }));
		expect(hooks.render).toHaveBeenCalledOnce();
	});
	it('does not request a full-frame resync when the client budget is unchanged', async () => {
		const api = await import('./websocket');
		cleanup = api.initializeWebSocket();
		Socket.instances.forEach((socket) => socket.onopen!());
		const socket = Socket.instances[1];
		socket.send.mockClear();
		// The 3D window controls call this on every slider step.
		api.setPreviewClientBudget(1000000);
		api.setPreviewClientBudget(1000000);
		expect(socket.send).not.toHaveBeenCalled();
		api.setPreviewClientBudget(500000);
		expect(socket.send).toHaveBeenCalledOnce();
		expect(socket.send).toHaveBeenLastCalledWith(
			JSON.stringify({ protocol: 2, maxPoints: 500000, resync: true })
		);
	});
	it('keeps render credit until a duplicate pending frame has actually drawn', async () => {
		let finish!: () => void;
		hooks.render.mockReturnValue(
			new Promise<boolean>((resolve) => {
				finish = () => resolve(true);
			})
		);
		const api = await import('./websocket');
		cleanup = api.initializeWebSocket();
		Socket.instances.forEach((socket) => socket.onopen!());
		api.setPreviewSurface({
			isConnected: true,
			clientWidth: 640,
			clientHeight: 480
		} as HTMLElement);
		api.parsePreviewMsgpack(previewFrame());
		await flushRender();
		const socket = Socket.instances[1];
		socket.send.mockClear();
		api.parsePreviewMsgpack(previewFrame());
		expect(socket.send).not.toHaveBeenCalled();
		finish();
		await flushRender();
		expect(socket.send).toHaveBeenCalledExactlyOnceWith(JSON.stringify({ ack: 1, revision: 7 }));
	});
	it('renders a new transport topology even when resync reuses the source sequence', async () => {
		hooks.render.mockResolvedValue(true);
		const api = await import('./websocket');
		cleanup = api.initializeWebSocket();
		Socket.instances.forEach((socket) => socket.onopen!());
		api.setPreviewSurface({
			isConnected: true,
			clientWidth: 640,
			clientHeight: 480
		} as HTMLElement);
		api.parsePreviewMsgpack(previewFrame());
		await flushRender();
		const socket = Socket.instances[1];
		socket.send.mockClear();
		api.parsePreviewMsgpack(previewFrame(8));
		expect(socket.send).not.toHaveBeenCalled();
		await flushRender();
		expect(hooks.render).toHaveBeenCalledTimes(2);
		expect(socket.send).toHaveBeenLastCalledWith(JSON.stringify({ ack: 1, revision: 8 }));
	});
	it('initializes once and tears down both sockets, retries and listeners', async () => {
		const { initializeWebSocket } = await import('./websocket');
		cleanup = initializeWebSocket();
		expect(initializeWebSocket()).toBe(cleanup);
		expect(Socket.instances).toHaveLength(2);
		cleanup();
		vi.advanceTimersByTime(60000);
		window.dispatchEvent(new Event('online'));
		document.dispatchEvent(new Event('visibilitychange'));
		expect(Socket.instances).toHaveLength(2);
		for (const socket of Socket.instances) expect(socket.close).toHaveBeenCalledOnce();
		expect(vi.getTimerCount()).toBe(0);
	});
	it('reports preview disconnection to an active transition', async () => {
		const { initializeWebSocket } = await import('./websocket');
		cleanup = initializeWebSocket();
		Socket.instances[1].onopen!();
		Socket.instances[1].onclose!();
		expect(hooks.fail).toHaveBeenCalledOnce();
	});
	it('never acknowledges a stale render as successful after teardown', async () => {
		let finishRender: (() => void) | undefined;
		hooks.render.mockReturnValue(
			new Promise<void>((resolve) => {
				finishRender = resolve;
			})
		);
		const api = await import('./websocket');
		const { meshState } = await import('./incoming/mesh');
		cleanup = api.initializeWebSocket();
		Socket.instances.forEach((socket) => socket.onopen!());
		meshState.set({
			Nx: 1,
			Ny: 1,
			Nz: 1,
			dx: 1,
			dy: 1,
			dz: 1,
			Tx: 1,
			Ty: 1,
			Tz: 1,
			PBCx: 0,
			PBCy: 0,
			PBCz: 0
		});
		api.setPreviewSurface({
			isConnected: true,
			clientWidth: 640,
			clientHeight: 480
		} as HTMLElement);
		const bytes = encode({
			sequence: 1,
			type: '3D',
			nComp: 3,
			vectorCount: 1,
			topologyRevision: 1,
			vectorFieldValues: [{ x: 1, y: 0, z: 0 }],
			vectorFieldPositions: [{ x: 0, y: 0, z: 0 }]
		});
		api.parsePreviewMsgpack(bytes.slice().buffer);
		for (const callback of frames.values()) callback(0);
		frames.clear();
		await vi.dynamicImportSettled();
		expect(hooks.render).toHaveBeenCalledOnce();
		cleanup();
		finishRender!();
		await Promise.resolve();
		await Promise.resolve();
		expect(hooks.finish).not.toHaveBeenCalled();
		expect(Socket.instances[1].send).toHaveBeenCalledTimes(1); // Only the subscription handshake.
	});
});
