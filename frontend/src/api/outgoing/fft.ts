import { post } from '$api/post';

export function postFftComponent(component: number) {
	return post('fft/component', { component });
}

export function postFftClear() {
	return post('fft/clear', {});
}

export function postFftMaxFrequency(maxFreqGHz: number) {
	return post('fft/max-frequency', { maxFreqGHz });
}
