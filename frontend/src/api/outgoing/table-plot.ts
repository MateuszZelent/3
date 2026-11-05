import { post } from '$api/post';

export function postXColumn(XColumn: string) {
	return post('tableplot/xcolumn', { XColumn });
}

export function postYColumn(YColumn: string) {
	return post('tableplot/ycolumn', { YColumn });
}

export function postAutoSaveInterval(autoSaveInterval: string) {
	return post('tableplot/autosave-interval', { autoSaveInterval });
}

export function postMaxPoints(inputFieldValue: string) {
	return post('tableplot/maxpoints', { maxPoints: parseInt(inputFieldValue) });
}

export function postStep(inputFieldValue: string) {
	return post('tableplot/step', { step: parseFloat(inputFieldValue) });
}
