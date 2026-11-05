import { writable } from 'svelte/store';
import type { EngineOperation } from '../acceptedCommand';

export interface Console extends EngineOperation {
	hist: string;
}

export const consoleState = writable<Console>({ hist: '' });
