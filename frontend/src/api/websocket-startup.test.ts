import { encode } from '@msgpack/msgpack';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { writable } from 'svelte/store';
import { previewState } from './incoming/preview';
import { meshState } from './incoming/mesh';
import {
	parseMsgpack,
	parsePreviewMsgpack,
	requestPreviewRender,
	setPreviewSurface
} from './websocket';
import { preview3D } from '$lib/preview/preview3D';
import { preview2D } from '$lib/preview/preview2D';

vi.mock('$lib/preview/preview3D', () => ({
	preview3D: vi.fn(),
	renderMode: writable('volume'),
	setRenderMode: vi.fn()
}));
vi.mock('$lib/preview/preview2D', () => ({ preview2D: vi.fn() }));

const frames: FrameRequestCallback[] = [];
vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
	frames.push(callback);
	return frames.length;
});
vi.stubGlobal('document', { hidden: false });
const mesh = {
	Nx: 7,
	Ny: 5,
	Nz: 3,
	dx: 5e-9,
	dy: 11e-9,
	dz: 17e-9,
	Tx: 35e-9,
	Ty: 55e-9,
	Tz: 51e-9,
	PBCx: 0,
	PBCy: 0,
	PBCz: 0
};
const emptyMesh = { ...mesh, Nx: 0, Ny: 0, Nz: 0, dx: 0, dy: 0, dz: 0 };
const surface = () => ({ isConnected: true, clientWidth: 640, clientHeight: 480 }) as HTMLElement;
function buffer(value: unknown) {
	return encode(value).slice().buffer as ArrayBuffer;
}
function data(type: string) {
	parsePreviewMsgpack(
		buffer({
			type,
			nComp: 3,
			quantity: 'm',
			vectorCount: type === '3D' ? 1 : 0,
			vectorFieldValues: type === '3D' ? [{ x: 1, y: 0, z: 0 }] : [],
			vectorFieldPositions: type === '3D' ? [{ x: 0, y: 0, z: 0 }] : [],
			scalarField: type === '2D' ? [[0, 0, 0.5]] : [],
			topologyRevision: 0
		})
	);
}
async function flush() {
	while (frames.length) {
		frames.shift()!(0);
		await vi.dynamicImportSettled();
		await Promise.resolve();
	}
}
beforeEach(async () => {
	setPreviewSurface(null);
	meshState.set(emptyMesh);
	previewState.update((state) => ({
		...state,
		type: '',
		nComp: 0,
		vectorCount: 0,
		vectorFieldValues: new Float32Array(),
		vectorFieldPositions: new Int32Array(),
		scalarField: []
	}));
	await flush();
	vi.clearAllMocks();
});
const orders = [
	'frame,mesh,mount',
	'frame,mount,mesh',
	'mesh,frame,mount',
	'mesh,mount,frame',
	'mount,mesh,frame',
	'mount,frame,mesh'
];
describe.each(['2D', '3D'])('%s initial preview', (type) => {
	it.each(orders)('renders the cached first frame in order %s', async (order) => {
		const renderer = type === '3D' ? preview3D : preview2D;
		const events = order.split(',');
		for (let i = 0; i < events.length; i++) {
			if (events[i] === 'frame') data(type);
			if (events[i] === 'mesh') parseMsgpack(buffer({ mesh }));
			if (events[i] === 'mount') setPreviewSurface(surface());
			await flush();
			if (i < 2) expect(renderer).not.toHaveBeenCalled();
		}
		expect(renderer).toHaveBeenCalledOnce();
	});
});
it('renders after a hidden surface gains dimensions without another data frame', async () => {
	const target = { isConnected: true, clientWidth: 0, clientHeight: 0 };
	setPreviewSurface(target as HTMLElement);
	parseMsgpack(buffer({ mesh }));
	data('3D');
	await flush();
	expect(preview3D).not.toHaveBeenCalled();
	target.clientWidth = 640;
	target.clientHeight = 480;
	requestPreviewRender();
	await flush();
	expect(preview3D).toHaveBeenCalledOnce();
});
it('redraws changed mesh geometry and ignores unchanged mesh telemetry', async () => {
	setPreviewSurface(surface());
	parseMsgpack(buffer({ mesh }));
	data('3D');
	await flush();
	vi.clearAllMocks();
	parseMsgpack(buffer({ mesh: { ...mesh, dx: 9e-9 } }));
	await flush();
	expect(preview3D).toHaveBeenCalledOnce();
	parseMsgpack(buffer({ mesh: { ...mesh, dx: 9e-9 } }));
	await flush();
	expect(preview3D).toHaveBeenCalledOnce();
});
it('does not initialize a renderer after its surface unmounts during a lazy import', async () => {
	setPreviewSurface(surface());
	parseMsgpack(buffer({ mesh }));
	data('3D');
	frames.shift()!(0);
	setPreviewSurface(null);
	await vi.dynamicImportSettled();
	await flush();
	expect(preview3D).not.toHaveBeenCalled();
});
