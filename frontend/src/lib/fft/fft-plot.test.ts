import { afterEach, describe, expect, it, vi } from 'vitest';
import { disposeFftCharts, plotSpectrogram, plotSpectrum } from './fft-plot';

afterEach(() => {
	disposeFftCharts();
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe('FFT chart lifecycle', () => {
	it('does not leave retry timers while chart tabs have no DOM nodes', () => {
		vi.useFakeTimers();
		vi.stubGlobal('document', { getElementById: () => null });

		plotSpectrum();
		plotSpectrogram();
		vi.advanceTimersByTime(60_000);

		expect(vi.getTimerCount()).toBe(0);
	});
});
