import { post } from '$api/post';

export function postSelectedRegion(selectedRegion: number) {
	return post('parameter/selected-region', { selectedRegion });
}
