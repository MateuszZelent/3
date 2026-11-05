import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { alert, setAlert } from './alert';

afterEach(() => {
	vi.useRealTimers();
	alert.set('');
});

describe('alert lifetime', () => {
	it('does not let an older timeout dismiss a newer message', () => {
		vi.useFakeTimers();

		setAlert('first');
		vi.advanceTimersByTime(5_000);
		setAlert('second');
		vi.advanceTimersByTime(5_000);
		expect(get(alert)).toBe('second');

		vi.advanceTimersByTime(5_000);
		expect(get(alert)).toBe('');
	});
});
