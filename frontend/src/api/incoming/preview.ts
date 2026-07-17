import { writable } from 'svelte/store';

export type VectorField = Float32Array;
export type VectorPositions = Int32Array;
export type ScalarField = Array<Array<number>>;

export interface Preview {
	quantity: string;
	unit: string;
	component: string;
	layer: number;
	allLayers: boolean;
	type: string;
	vectorFieldValues: VectorField;
	vectorFieldPositions: VectorPositions;
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
	xChosenSize: 0,
	yChosenSize: 0,
	appliedXChosenSize: 0,
	appliedYChosenSize: 0,
	appliedLayerStride: 1,
	autoScaleEnabled: true,
	autoDownscaled: false,
	autoDownscaleMessage: ''
});
