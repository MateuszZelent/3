import type { PreviewRegion } from '$lib/preview/preview3DRegion';
import { writable } from 'svelte/store';

export type VectorField = Float32Array;
export type VectorPositions = Int32Array;
export type ScalarField = Array<Array<number>>;

export interface Preview {
	region?: PreviewRegion;
	regionActive?: boolean;
	transportSampling?: number;
	serverVectorCount?: number;
	sequence?: number;
	step?: number;
	timestamp?: number;
	normScale?: number;
	fixedScale?: number;
	invalidCount?: number;
	captureMs?: number;
	processMs?: number;
	hardLimit?: number;
	quantity: string;
	unit: string;
	component: string;
	layer: number;
	plane?: 'xy' | 'yz' | 'xz';
	sliceIndex?: number;
	planeUPossibleSizes?: number[];
	planeVPossibleSizes?: number[];
	planeUChosenSize?: number;
	planeVChosenSize?: number;
	appliedPlaneUSize?: number;
	appliedPlaneVSize?: number;
	allLayers: boolean;
	type: string;
	vectorFieldValues: VectorField;
	vectorFieldPositions: VectorPositions;
	vectorOccupancy?: Uint8Array;
	vectorCount: number;
	topologyRevision: number;
	scalarField: ScalarField;
	min: number;
	max: number;
	refresh: boolean;
	nComp: number;

	maxPoints: number;
	dataPointsCount: number;
	xPossibleSizes: number[];
	yPossibleSizes: number[];
	zPossibleSizes: number[];
	zChosenSize: number;
	appliedZChosenSize: number;
	xChosenSize: number;
	yChosenSize: number;
	appliedXChosenSize: number;
	appliedYChosenSize: number;
	appliedLayerStride: number;
	autoScaleEnabled: boolean;
	autoDownscaled: boolean;
	autoDownscaleMessage: string;
}

export const previewState = writable<Preview>({
	quantity: '',
	unit: '',
	component: '',
	layer: 0,
	plane: 'xy',
	sliceIndex: 0,
	allLayers: false,
	maxPoints: 0,
	type: '',
	vectorFieldValues: new Float32Array(),
	vectorFieldPositions: new Int32Array(),
	vectorCount: 0,
	topologyRevision: 0,
	scalarField: [],
	min: 0,
	max: 0,
	refresh: false,
	nComp: 0,
	dataPointsCount: 0,
	xPossibleSizes: [],
	yPossibleSizes: [],
	zPossibleSizes: [],
	zChosenSize: 0,
	appliedZChosenSize: 0,
	xChosenSize: 0,
	yChosenSize: 0,
	appliedXChosenSize: 0,
	appliedYChosenSize: 0,
	appliedLayerStride: 1,
	autoScaleEnabled: true,
	autoDownscaled: false,
	autoDownscaleMessage: ''
});
