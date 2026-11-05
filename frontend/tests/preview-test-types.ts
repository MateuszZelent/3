import type { Camera, WebGLRenderer, Scene } from 'three';
import type { VectorMesh } from '../src/lib/preview/vectorInstances';

export type PreviewDisplay = {
	mesh: VectorMesh;
	scene: Scene;
	renderer: WebGLRenderer;
	camera: Camera;
	controls: { target: { toArray(): number[] } };
};

export type VolumeSnapshot = {
	count: number;
	depths: number[];
	scale: number[];
	solid: number;
	topo: number;
	color: number;
	opaque: boolean;
	opacity: number;
	positions: number[];
	triangles: number;
};
