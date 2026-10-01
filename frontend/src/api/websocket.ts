import { markPreviewRendering, finishPreviewTransition } from './previewTransition';
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

export let connected = writable(false);
export let connectionState = writable<ConnectionState>('disconnected');
export const previewConnected = writable(false);
export const previewError = writable('');
let previewSocket: WebSocket | null = null;
let previewVisible = true;
// Receive the full server preview by default. Auto-adjust owns the default limit.
export const previewClientBudget = writable(1000000);
let lastSequence = 0;
let pendingAck: { sequence: number; revision: number } | null = null;
let previewRenderScheduled = false;
let previewRendering = false;
let previewRenderAgain = false;
let tableRenderScheduled = false;
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

	const {
		vectorValuesBinary: _values,
		vectorPositionsBinary: _positions,
		vectorFieldValues: _legacyValues,
		vectorFieldPositions: _legacyPositions,
		...metadata
	} = msg;
	return {
		...metadata,
		vectorFieldValues: values,
		vectorFieldPositions: positions,
		vectorCount,
		topologyRevision: revision
	} as Preview;
}

async function renderPreview() {
	if (get(previewState).type === '3D') {
		const { preview3D, setRenderMode, renderMode } = await import('$lib/preview/preview3D');
		if (get(previewState).nComp === 1 && get(renderMode) !== 'volume') setRenderMode('volume');
		await preview3D();
		return;
	}

	const { preview2D } = await import('$lib/preview/preview2D');
	await preview2D();
}

async function renderTablePlot() {
	const { plotTable } = await import('$lib/table-plot/table-plot');
	await plotTable();
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
	requestAnimationFrame(() => {
		previewRenderScheduled = false;
		if (!previewVisible || document.hidden) {
			ackPreview();
			return;
		}
		previewRendering = true;
		const rendered = get(previewState);
		const ack = pendingAck;
		markPreviewRendering(rendered);
		void renderPreview()
			.then(() => {
				finishPreviewTransition(rendered);
				if (pendingAck === ack) ackPreview();
			})
			.catch((error) => {
				previewError.set(String(error));
				if (pendingAck === ack) ackPreview();
			})
			.finally(() => {
				previewRendering = false;
				if (previewRenderAgain) {
					previewRenderAgain = false;
					schedulePreviewRender();
				}
			});
	});
}

function scheduleTableRender() {
	if (tableRenderScheduled) {
		return;
	}
	tableRenderScheduled = true;
	requestAnimationFrame(() => {
		tableRenderScheduled = false;
		void renderTablePlot();
	});
}

function connectWS(
	wsUrl: string,
	onOpen: () => void,
	onClose: () => void,
	onMessage: (data: ArrayBuffer) => void
) {
	const retryInterval = 1000;
	let ws: WebSocket | null = null;

	function connect() {
		console.debug('Connecting to WebSocket server at', wsUrl);
		ws = new WebSocket(wsUrl);
		ws.binaryType = 'arraybuffer';

		ws.onopen = function () {
			if (wsUrl.endsWith('/preview')) {
				previewSocket = ws;
				lastSequence = 0;
				cachedTopologyRevision = -1;
				cachedVectorPositions = new Int32Array();
				ws?.send(
					JSON.stringify({
						protocol: 2,
						maxPoints: get(previewClientBudget),
						subscribe: previewVisible && !document.hidden
					})
				);
			}
			onOpen();
		};

		ws.onmessage = function (event) {
			try {
				onMessage(event.data as ArrayBuffer);
			} catch (error) {
				previewError.set(String(error));
				if (wsUrl.endsWith('/preview')) ws?.send(JSON.stringify({ resync: true }));
			}
		};

		ws.onclose = function () {
			if (wsUrl.endsWith('/preview')) previewSocket = null;
			onClose();
			console.debug(
				'WebSocket closed. Attempting to reconnect in ' + retryInterval / 1000 + ' seconds...'
			);
			ws = null;
			setTimeout(connect, retryInterval);
		};

		ws.onerror = function (event) {
			console.error('WebSocket encountered error:', event);
			if (ws) {
				ws.close();
			}
		};
	}

	try {
		connect();
	} catch (err) {
		console.error(
			'WebSocket connection failed:',
			err,
			'Retrying in ' + retryInterval / 1000 + ' seconds...'
		);
		setTimeout(connect, retryInterval);
	}
}

export function initializeWebSocket() {
	document.addEventListener('visibilitychange', () => setPreviewVisible(previewVisible));
	connectWS(
		'./ws',
		() => {
			connected.set(true);
			connectionState.set('connected');
		},
		() => {
			connected.set(false);
			connectionState.update((state) => (state === 'connected' ? 'reconnecting' : 'disconnected'));
		},
		parseMsgpack
	);
	connectWS(
		'./ws/preview',
		() => previewConnected.set(true),
		() => previewConnected.set(false),
		parsePreviewMsgpack
	);
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
		meshState.set(msg.mesh);
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
			coreEnabled: msg.tablePlot.coreEnabled ?? false
		});
		scheduleTableRender();
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
	if (sequence > 0 && sequence <= lastSequence) return;
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

export function setPreviewClientBudget(maxPoints: number) {
	previewClientBudget.set(maxPoints);
	if (previewSocket?.readyState === WebSocket.OPEN)
		previewSocket.send(JSON.stringify({ protocol: 2, maxPoints, resync: true }));
}
