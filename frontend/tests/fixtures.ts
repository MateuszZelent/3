import { expect, test as base } from '@playwright/test';
import type { Page } from '@playwright/test';
import type { Preview } from '../src/api/incoming/preview';

/** UI tests must never send commands to an unrelated local solver. */
export const test = base.extend<{ isolatedBackend: void }>({
	isolatedBackend: [
		async ({ page, baseURL }, use) => {
			const unexpected: string[] = [];
			const errors: string[] = [];
			page.on('pageerror', (error) => errors.push(error.message));
			await page.routeWebSocket(/\/ws/, () => {});
			await page.route(`${baseURL}/api/**`, (route) => {
				unexpected.push(`${route.request().method()} ${new URL(route.request().url()).pathname}`);
				return route.fulfill({
					status: 501,
					json: { error: 'Unexpected API request in isolated UI test' }
				});
			});
			await page.route('**/api/demag/progress', (route) =>
				route.fulfill({
					json: {
						id: 0,
						active: false,
						failed: false,
						stage: '',
						percent: 0,
						startedAt: 0,
						logs: []
					}
				})
			);
			await page.route('https://fonts.googleapis.com/**', (route) =>
				route.fulfill({ contentType: 'text/css', body: '' })
			);
			await use();
			expect(unexpected, 'All backend commands must have an explicit test fixture').toEqual([]);
			expect(errors, 'The UI must not produce unhandled exceptions').toEqual([]);
		},
		{ auto: true }
	]
});

export { expect };

/** Publish a versioned server result through the actual render/ACK scheduler. */
export async function publishPreviewConfig(
	page: Page,
	patch: Record<string, unknown>,
	configRevision: number
) {
	await page.evaluate(
		async ({ patch, configRevision }) => {
			const previewPath = '/src/api/incoming/preview.ts';
			const socketPath = '/src/api/websocket.ts';
			const { previewState } = await import(previewPath);
			const { requestPreviewRender } = await import(socketPath);
			previewState.update((state: Preview) => ({
				...state,
				...patch,
				configRevision,
				sequence: (state.sequence ?? 0) + 1
			}));
			requestPreviewRender();
		},
		{ patch, configRevision }
	);
}
