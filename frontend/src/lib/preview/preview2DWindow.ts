import { writable } from 'svelte/store';

export type WindowRange = [number, number];
export type PlaneWindow = { u: WindowRange; v: WindowRange };
export const preview2DWindow = writable<PlaneWindow>({ u: [0, 1], v: [0, 1] });

export function clampWindow(range: WindowRange, minimum = 0.001): WindowRange {
	const width = Math.min(1, Math.max(minimum, range[1] - range[0]));
	const start = Math.min(1 - width, Math.max(0, range[0]));
	return [start, start + width];
}

export function moveWindow(range: WindowRange, delta: number): WindowRange {
	return clampWindow([range[0] + delta, range[1] + delta], range[1] - range[0]);
}
