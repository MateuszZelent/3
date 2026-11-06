import {
	markPreviewRendering,
	finishPreviewTransition,
	failPreviewTransition,
	cancelPreviewTransition,
	setPreviewRecoveryHandler
} from './previewTransition';
import { connectWebSocket } from './websocketConnection';
import { decode } from '@msgpack/msgpack';

import { type Preview, previewState } from './incoming/preview';
import { type Header, headerState } from './incoming/header';
import { type Solver, solverState } from './incoming/solver';
import { type Console, consoleState } from './incoming/console';
import { type Mesh, meshState } from './incoming/mesh';
import { type Parameters, parametersState, sortFieldsByName } from './incoming/parameters';
import { type TablePlot, tablePlotState } from './incoming/table-plot';
import { get, writable } from 'svelte/store';
import { metricsState, type Metrics } from './incoming/metrics';
import { fftState, type FftData } from './incoming/fft';
import type { ConnectionState } from '$lib/ui/types';

type MainUpdate = {
	console?: Console;
	header?: Header;
	mesh?: Mesh;
	parameters?: Parameters;
	solver?: Solver;
	tablePlot?: TablePlot;
	preview?: Preview | null;
	metrics?: Metrics;
	fft?: FftData;
};

type LegacyVectorField = Array<{ x: number; y: number; z: number }>;
type PreviewWire = Omit<Preview, 'vectorFieldValues' | 'vectorFieldPositions'> & {
	vectorFieldValues?: LegacyVectorField | Float32Array;
	vectorFieldPositions?: LegacyVectorField | Int32Array;
	vectorValuesBinary?: Uint8Array;
	vectorPositionsBinary?: Uint8Array;
};

export const connected = writable(false);
export const connectionState = writable<ConnectionState>('disconnected');
export const previewConnected = writable(false);
export const previewError = writable('');
let previewSocket: WebSocket | null = null;
let previewVisible = true;
// Receive the full server preview by default. Auto-adjust owns the default limit.
export const previewClientBudget = writable(1000000);
let lastSequence = 0;
let pendingAck: { sequence: number; revision: number } | null = null;
let previewSurface: HTMLElement | null = null;
let previewRenderScheduled = false;
let previewRendering = false;
let previewRenderAgain = false;
let previewFrame: number | undefined;
let renderGeneration = 0;
let disposeConnections: (() => void) | null = null;
let cachedVectorPositions: Int32Array<ArrayBufferLike> = new Int32Array();
let cachedTopologyRevision = -1;

function typedArrayView<T extends Float32Array | Int32Array>(
	bytes: Uint8Array,
	ctor: {
		new (buffer: ArrayBufferLike, byteOffset: number, length: number): T;
	}
): T {
	if (bytes.byteLength % 12 !== 0) throw new Error('Incomplete vector buffer');
	const byteLength = bytes.byteLength;
	if (bytes.byteOffset % 4 === 0) {
		return new ctor(bytes.buffer, bytes.byteOffset, byteLength / 4);
	}
	const aligned = bytes.slice(0, byteLength);
	return new ctor(aligned.buffer, aligned.byteOffset, byteLength / 4);
}

function flattenLegacyVectors(values: LegacyVectorField) {
	const result = new Float32Array(values.length * 3);
	for (let i = 0; i < values.length; i++) {
		result[i * 3] = values[i].x;
		result[i * 3 + 1] = values[i].y;
		result[i * 3 + 2] = values[i].z;
	}
	return result;
}

function flattenLegacyPositions(values: LegacyVectorField) {
	const result = new Int32Array(values.length * 3);
	for (let i = 0; i < values.length; i++) {
		result[i * 3] = values[i].x;
		result[i * 3 + 1] = values[i].y;
		result[i * 3 + 2] = values[i].z;
	}
	return result;
}

function normalizePreview(msg: PreviewWire): Preview {
	const revision = Number(msg.topologyRevision ?? 0);
	let candidatePositions = cachedVectorPositions;
	let candidateRevision = cachedTopologyRevision;
	let values: Float32Array<ArrayBufferLike> = new Float32Array();
	if (msg.vectorValuesBinary instanceof Uint8Array) {
		values = typedArrayView(msg.vectorValuesBinary, Float32Array);
	} else if (msg.vectorFieldValues instanceof Float32Array) {
		values = msg.vectorFieldValues;
	} else if (Array.isArray(msg.vectorFieldValues)) {
		values = flattenLegacyVectors(msg.vectorFieldValues);
	}

	if (msg.vectorPositionsBinary instanceof Uint8Array) {
		candidatePositions = typedArrayView(msg.vectorPositionsBinary, Int32Array);
		candidateRevision = revision;
	} else if (msg.vectorFieldPositions instanceof Int32Array) {
		candidatePositions = msg.vectorFieldPositions;
		candidateRevision = revision;
	} else if (Array.isArray(msg.vectorFieldPositions)) {
		candidatePositions = flattenLegacyPositions(msg.vectorFieldPositions);
		candidateRevision = revision;
	} else if (candidateRevision !== revision) {
		candidatePositions = new Int32Array();
		candidateRevision = revision;
	}

	if (
		msg.vectorOccupancy !== undefined &&
		msg.vectorOccupancy !== null &&
		(!(msg.vectorOccupancy instanceof Uint8Array) ||
			msg.vectorOccupancy.length !== values.length / 3)
	)
		throw new Error('Invalid volume occupancy');
	const declared = Number(msg.vectorCount ?? values.length / 3);
	if (!Number.isSafeInteger(declared) || declared < 0 || declared > 1000000)
		throw new Error('Invalid preview count');
	if (
		msg.type === '3D' &&
		(values.length !== declared * 3 || candidatePositions.length !== declared * 3)
	)
		throw new Error('Missing preview topology or mismatched vector count');
	const vectorCount = Math.min(
		Number(msg.vectorCount ?? values.length / 3),
		Math.floor(values.length / 3),
		Math.floor(candidatePositions.length / 3)
	);
	cachedVectorPositions = candidatePositions;
	cachedTopologyRevision = candidateRevision;
	const positions = vectorCount > 0 ? candidatePositions : new Int32Array();

	const metadata = { ...msg };
	delete metadata.vectorValuesBinary;
	delete metadata.vectorPositionsBinary;
	delete metadata.vectorFieldValues;
	delete metadata.vectorFieldPositions;
	return {
		...metadata,
		vectorFieldValues: values,
		vectorFieldPositions: positions,
		vectorCount,
		topologyRevision: revision
	} as Preview;
}

function previewCanRender() {
	const mesh = get(previewState).mesh ?? get(meshState);
	return (
		!!previewSurface?.isConnected &&
		previewSurface.clientWidth > 0 &&
		previewSurface.clientHeight > 0 &&
		[mesh.Nx, mesh.Ny, mesh.Nz, mesh.dx, mesh.dy, mesh.dz].every((n) => Number.isFinite(n) && n > 0)
	);
}

// A frame may arrive before the lazy visualization component or mesh metadata.
// Retain it in the store and render when both dependencies become available.
function invalidatePreviewRender() {
	renderGeneration++;
	if (previewFrame !== undefined && typeof cancelAnimationFrame === 'function')
		cancelAnimationFrame(previewFrame);
	previewFrame = undefined;
	previewRenderScheduled = false;
	previewRendering = false;
	previewRenderAgain = false;
}
export function setPreviewSurface(surface: HTMLElement | null) {
	if (previewSurface !== surface) invalidatePreviewRender();
	previewSurface = surface;
	if (surface) schedulePreviewRender();
	else ackPreview();
}
export function requestPreviewRender() {
	schedulePreviewRender();
}

async function renderPreview() {
	if (get(previewState).type === '3D') {
		const { preview3D, setRenderMode, renderMode } = await import('$lib/preview/preview3D');
		if (!previewCanRender() || get(previewState).type !== '3D') return null;
		const state = get(previewState);
		markPreviewRendering(state);
		if (state.nComp === 1 && get(renderMode) !== 'volume')
			setRenderMode('volume', { ensureAllLayers: false });
		const rendered: unknown = await preview3D();
		return rendered === false ? null : state;
	}

	const { preview2D } = await import('$lib/preview/preview2D');
	if (!previewCanRender() || get(previewState).type !== '2D') return null;
	const state = get(previewState);
	markPreviewRendering(state);
	await preview2D();
	return state;
}

function schedulePreviewRender() {
	if (previewRendering) {
		previewRenderAgain = true;
		return;
	}
	if (previewRenderScheduled) {
		return;
	}
	previewRenderScheduled = true;
	const generation = renderGeneration;
	previewFrame = requestAnimationFrame(() => {
		if (generation !== renderGeneration) return;
		previewFrame = undefined;
		previewRenderScheduled = false;
		if (!previewVisible || document.hidden || !previewCanRender()) {
			ackPreview();
			return;
		}
		previewRendering = true;
		const ack = pendingAck;
		void renderPreview()
			.then((rendered) => {
				if (generation !== renderGeneration) return;
				if (rendered && previewCanRender()) finishPreviewTransition(rendered);
				if (pendingAck === ack) ackPreview();
			})
			.catch((error) => {
				if (generation !== renderGeneration) return;
				previewError.set(String(error));
				failPreviewTransition(error);
				if (pendingAck === ack) ackPreview();
			})
			.finally(() => {
				if (generation !== renderGeneration) return;
				previewRendering = false;
				if (previewRenderAgain) {
					previewRenderAgain = false;
					schedulePreviewRender();
				}
			});
	});
}

export function requestPreviewResync() {
	if (previewSocket?.readyState === WebSocket.OPEN)
		previewSocket.send(JSON.stringify({ resync: true }));
}

function connectWS(
	wsUrl: string,
	onOpen: () => void,
	onClose: () => void,
	onMessage: (data: ArrayBuffer) => void
) {
	const isPreview = wsUrl.endsWith('/preview');
	let current: WebSocket | null = null;
	return connectWebSocket(wsUrl, {
		open: (socket) => {
			current = socket;
			if (isPreview) {
				previewSocket = socket;
				lastSequence = 0;
				pendingAck = null;
				cachedTopologyRevision = -1;
				cachedVectorPositions = new Int32Array();
				socket.send(
					JSON.stringify({
						protocol: 2,
						maxPoints: get(previewClientBudget),
						subscribe: previewVisible && !document.hidden
					})
				);
			}
			onOpen();
		},
		close: () => {
			current = null;
			if (isPreview) {
				previewSocket = null;
				pendingAck = null;
				failPreviewTransition(new Error('Preview connection lost. Retry when connected.'));
			}
			onClose();
		},
		message: (data) => {
			try {
				onMessage(data);
			} catch (error) {
				previewError.set(String(error));
				if (isPreview) {
					failPreviewTransition(error);
					if (current?.readyState === WebSocket.OPEN) requestPreviewResync();
				}
			}
		}
	});
}

export function initializeWebSocket(): () => void {
	if (disposeConnections) return disposeConnections;
	const visibilityChanged = () => setPreviewVisible(previewVisible);
	document.addEventListener('visibilitychange', visibilityChanged);
	setPreviewRecoveryHandler(() => {
		requestPreviewResync();
		requestPreviewRender();
	});
	const disposeMain = connectWS(
		'./ws',
		() => {
			connected.set(true);
			connectionState.set('connected');
		},
		() => {
			connected.set(false);
			connectionState.update((state) => (state === 'disconnected' ? state : 'reconnecting'));
		},
		parseMsgpack
	);
	const disposePreview = connectWS(
		'./ws/preview',
		() => previewConnected.set(true),
		() => previewConnected.set(false),
		parsePreviewMsgpack
	);
	const dispose = () => {
		if (disposeConnections !== dispose) return;
		disposeConnections = null;
		document.removeEventListener('visibilitychange', visibilityChanged);
		setPreviewRecoveryHandler(null);
		cancelPreviewTransition({ restore: false });
		disposeMain();
		disposePreview();
		invalidatePreviewRender();
		previewSurface = null;
		previewSocket = null;
		pendingAck = null;
		connected.set(false);
		previewConnected.set(false);
		connectionState.set('disconnected');
	};
	disposeConnections = dispose;
	return dispose;
}

export function parseMsgpack(data: ArrayBuffer) {
	const msg = decode(new Uint8Array(data)) as MainUpdate;

	if (msg.console) {
		consoleState.set(msg.console);
	}
	if (msg.header) {
		headerState.set(msg.header);
	}
	if (msg.mesh) {
		const previous = get(meshState);
		const geometryChanged = (['Nx', 'Ny', 'Nz', 'dx', 'dy', 'dz'] as const).some(
			(key) => previous[key] !== msg.mesh![key]
		);
		meshState.set(msg.mesh);
		if (geometryChanged) schedulePreviewRender();
	}
	if (msg.parameters) {
		parametersState.set(msg.parameters);
		sortFieldsByName();
	}
	if (msg.solver) {
		solverState.set(msg.solver);
	}
	if (msg.tablePlot) {
		tablePlotState.set({
			...msg.tablePlot,
			columns: msg.tablePlot.columns ?? [],
			data: msg.tablePlot.data ?? [],
			corePos: msg.tablePlot.corePos ?? null,
			coreEnabled: msg.tablePlot.coreEnabled ?? false,
			coreMode: msg.tablePlot.coreMode ?? '',
			coreStatus: msg.tablePlot.coreStatus ?? ''
		});
	}
	if (msg.preview) {
		acceptPreview(msg.preview as PreviewWire);
	}
	if (msg.metrics) {
		metricsState.set(msg.metrics);
	}
	if (msg.fft) {
		fftState.set(msg.fft);
	}
}

export function parsePreviewMsgpack(data: ArrayBuffer) {
	const msg = decode(new Uint8Array(data)) as PreviewWire;
	acceptPreview(msg);
}

function acceptPreview(msg: PreviewWire) {
	const sequence = Number(msg.sequence ?? 0);
	if (sequence > 0 && sequence < lastSequence) return;
	if (
		sequence > 0 &&
		sequence === lastSequence &&
		Number(msg.topologyRevision ?? 0) === cachedTopologyRevision
	) {
		// Resync can replay an already accepted frame and reset the server's
		// credit. Return that credit without replacing a newer pending ACK.
		// A frame still drawing keeps its original render-completion ACK.
		if (pendingAck?.sequence !== sequence && previewSocket?.readyState === WebSocket.OPEN)
			previewSocket.send(JSON.stringify({ ack: sequence, revision: cachedTopologyRevision }));
		return;
	}
	// A different transport profile can reuse the source sequence. Normalize
	// and draw its topology before acknowledging that profile's revision.
	const state = normalizePreview(msg);
	if (sequence > 0) lastSequence = sequence;
	previewState.set(state);
	previewError.set('');
	pendingAck = { sequence, revision: state.topologyRevision };
	schedulePreviewRender();
}
function ackPreview() {
	if (!pendingAck) return;
	const ack = pendingAck;
	pendingAck = null;
	if (previewSocket?.readyState === WebSocket.OPEN)
		previewSocket.send(JSON.stringify({ ack: ack.sequence, revision: ack.revision }));
}
export function setPreviewVisible(visible: boolean) {
	previewVisible = visible;
	if (previewSocket?.readyState === WebSocket.OPEN)
		previewSocket.send(JSON.stringify({ subscribe: visible && !document.hidden }));
	if (visible && !document.hidden) schedulePreviewRender();
	else ackPreview();
}

// A resync makes the server resend the whole latest frame (up to ~24 MB), so
// only send it when the budget actually changes. Callers such as the 3D window
// controls invoke this on every slider step; the socket open handshake already
// carries the current budget.
export function setPreviewClientBudget(maxPoints: number) {
	if (get(previewClientBudget) === maxPoints) return;
	previewClientBudget.set(maxPoints);
	if (previewSocket?.readyState === WebSocket.OPEN)
		previewSocket.send(JSON.stringify({ protocol: 2, maxPoints, resync: true }));
}
