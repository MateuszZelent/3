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
let previewRenderScheduled = false;
let tableRenderScheduled = false;
let cachedVectorPositions: Int32Array<ArrayBufferLike> = new Int32Array();
let cachedTopologyRevision = -1;

function typedArrayView<T extends Float32Array | Int32Array>(
	bytes: Uint8Array,
	ctor: { new (buffer: ArrayBufferLike, byteOffset: number, length: number): T }
): T {
	const byteLength = bytes.byteLength - (bytes.byteLength % 4);
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
	let values: Float32Array<ArrayBufferLike> = new Float32Array();
	if (msg.vectorValuesBinary instanceof Uint8Array) {
		values = typedArrayView(msg.vectorValuesBinary, Float32Array);
	} else if (msg.vectorFieldValues instanceof Float32Array) {
		values = msg.vectorFieldValues;
	} else if (Array.isArray(msg.vectorFieldValues)) {
		values = flattenLegacyVectors(msg.vectorFieldValues);
	}

	if (msg.vectorPositionsBinary instanceof Uint8Array) {
		cachedVectorPositions = typedArrayView(msg.vectorPositionsBinary, Int32Array);
		cachedTopologyRevision = revision;
	} else if (msg.vectorFieldPositions instanceof Int32Array) {
		cachedVectorPositions = msg.vectorFieldPositions;
		cachedTopologyRevision = revision;
	} else if (Array.isArray(msg.vectorFieldPositions)) {
		cachedVectorPositions = flattenLegacyPositions(msg.vectorFieldPositions);
		cachedTopologyRevision = revision;
	} else if (cachedTopologyRevision !== revision) {
		cachedVectorPositions = new Int32Array();
		cachedTopologyRevision = revision;
	}

	const vectorCount = Math.min(
		Number(msg.vectorCount ?? values.length / 3),
		Math.floor(values.length / 3),
		Math.floor(cachedVectorPositions.length / 3)
	);
	const positions = vectorCount > 0 ? cachedVectorPositions : new Int32Array();

	return {
		...msg,
		vectorFieldValues: values,
		vectorFieldPositions: positions,
		vectorCount,
		topologyRevision: revision
	} as Preview;
}

async function renderPreview() {
	if (get(previewState).type === '3D') {
		const { preview3D } = await import('$lib/preview/preview3D');
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
	if (previewRenderScheduled) {
		return;
	}
	previewRenderScheduled = true;
	requestAnimationFrame(() => {
		previewRenderScheduled = false;
		void renderPreview();
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
			onOpen();
		};

		ws.onmessage = function (event) {
			onMessage(event.data as ArrayBuffer);
			ws?.send('ok');
		};

		ws.onclose = function () {
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
		() => undefined,
		() => undefined,
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
		previewState.set(normalizePreview(msg.preview as PreviewWire));
		schedulePreviewRender();
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
	previewState.set(normalizePreview(msg));
	schedulePreviewRender();
}
