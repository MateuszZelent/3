import { post } from '$api/post';

export function postResetError() {
	return post('metrics/reset-error', {});
}
