import { post } from '$api/post';

export function postComponent(component: string) {
	post('preview/component', { component });
}

export function postQuantity(quantity: string) {
	post('preview/quantity', { quantity });
}

export function postLayer(layer: number) {
	post('preview/layer', { layer });
}

export function postXChosenSize(xChosenSize: number) {
	post('preview/XChosenSize', { xChosenSize });
}
export function postYChosenSize(yChosenSize: number) {
	post('preview/YChosenSize', { yChosenSize });
}

export function postRefresh() {
	post('preview/refresh', {});
}

export function postAllLayers(allLayers: boolean) {
	post('preview/allLayers', { allLayers });
}

export function postAutoScaleEnabled(autoScaleEnabled: boolean) {
	post('preview/autoScaleEnabled', { autoScaleEnabled });
}

export function postZChosenSize(zChosenSize: number) {
	post('preview/ZChosenSize', { zChosenSize });
}

export function postMaxPoints(maxPoints: number) {
	return post('preview/maxpoints', { maxPoints });
}
export function postScale(scale: number) {
	return post('preview/scale', { scale });
}

export function postFullResolution() {
	return post('preview/fullResolution', {});
}

export function postSection(section: {
	plane?: 'xy' | 'yz' | 'xz';
	mode?: 'single' | 'average';
	sliceIndex?: number;
}) {
	return post('preview/section', section);
}
export function postPlaneResolution(resolution: { uSize?: number; vSize?: number }) {
	return post('preview/planeResolution', resolution);
}
