import { afterEach, describe, expect, it, vi } from 'vitest';
import { disposeTablePlot, plotTable } from './table-plot';

afterEach(() => {
	disposeTablePlot();
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe('table plot lifecycle', () => {
	it('does not poll for a DOM node when table data has not created the chart element', () => {
		vi.useFakeTimers();
		vi.stubGlobal('document', { getElementById: () => null });

		plotTable();
		vi.advanceTimersByTime(60_000);

		expect(vi.getTimerCount()).toBe(0);
	});
});
