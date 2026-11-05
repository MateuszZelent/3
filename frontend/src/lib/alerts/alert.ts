// svelte store that manages the alerts
import { writable } from 'svelte/store';

export const alert = writable('');

let dismissTimer: ReturnType<typeof setTimeout> | undefined;

export function setAlert(message: string) {
	if (dismissTimer !== undefined) clearTimeout(dismissTimer);
	alert.set(message);
	dismissTimer = setTimeout(() => {
		alert.set('');
		dismissTimer = undefined;
	}, 10000);
}
