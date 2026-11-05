import { expect, test } from './fixtures';

test('console scrolls to the final entry after initial and subsequent history updates', async ({
	page
}) => {
	await page.goto('/');
	const output = page.locator('.console-shell__output');
	await page.evaluate(async () => {
		const path = '/src/api/incoming/console.ts';
		const { consoleState } = await import(path);
		consoleState.set({ hist: Array.from({ length: 100 }, (_, i) => `line ${i}`).join('\n') });
	});
	await expect
		.poll(() =>
			output.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)
		)
		.toBeLessThan(2);
	await output.evaluate((element) => (element.scrollTop = 0));
	await page.evaluate(async () => {
		const path = '/src/api/incoming/console.ts';
		const { consoleState } = await import(path);
		consoleState.update((state: { hist: string }) => ({ hist: state.hist + '\nnew final entry' }));
	});
	await expect(output).toContainText('new final entry');
	await expect
		.poll(() =>
			output.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)
		)
		.toBeLessThan(2);
});
