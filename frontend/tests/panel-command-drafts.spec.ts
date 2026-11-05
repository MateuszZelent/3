import { expect, test } from './fixtures';
import type { Console as ConsoleState } from '../src/api/incoming/console';
import type { Solver } from '../src/api/incoming/solver';
import type { TablePlot } from '../src/api/incoming/table-plot';

test('console keeps a failed draft and suppresses duplicate submissions while pending', async ({
	page
}) => {
	let requests = 0;
	let breakRequests = 0;
	let accept!: () => void;
	const acceptance = new Promise<void>((resolve) => {
		accept = resolve;
	});
	await page.route('**/api/console/command', async (route) => {
		requests++;
		if (requests === 1) {
			await route.fulfill({ status: 503, json: { error: 'temporary failure' } });
			return;
		}
		await acceptance;
		await route.fulfill({ status: 202, json: { operationId: 1, status: 'pending' } });
	});
	await page.route('**/api/solver/break', async (route) => {
		breakRequests++;
		await route.fulfill({ status: 204 });
	});

	await page.goto('/');
	await page.evaluate(async () => {
		const websocketPath = '/src/api/websocket.ts';
		const solverPath = '/src/api/incoming/solver.ts';
		const { connectionState } = await import(websocketPath);
		const { solverState } = await import(solverPath);
		connectionState.set('connected');
		solverState.update((state: Solver) => ({
			...state,
			operationId: 7,
			operationStatus: 'running'
		}));
	});

	const input = page.getByPlaceholder('Run a command. Use ↑ and ↓ for history.');
	await input.fill('print(1)');
	await input.press('Enter');
	await expect.poll(() => requests).toBe(1);
	await expect(page.locator('.toast-stack[role="status"]')).toContainText('temporary failure');
	await expect(input).toHaveValue('print(1)');

	await input.press('Enter');
	await expect(page.getByRole('button', { name: 'Sending…' })).toBeDisabled();
	const consolePanel = page.locator('.ui-panel[data-panel="console"]');
	const consoleBreak = consolePanel.getByRole('button', { name: 'Break' });
	await expect(consoleBreak).toBeEnabled();
	await consoleBreak.click();
	await expect.poll(() => breakRequests).toBe(1);
	await input.press('Enter');
	await expect.poll(() => requests).toBe(2);
	await page.waitForTimeout(100);
	await expect.poll(() => requests).toBe(2);
	await expect(input).toHaveValue('print(1)');
	accept();
	await expect(input).toHaveValue('');
	await expect(consoleBreak).toBeVisible();
	const { operationId, operationStatus } = await page.evaluate(async () => {
		const consolePath = '/src/api/incoming/console.ts';
		const solverPath = '/src/api/incoming/solver.ts';
		const { consoleState } = await import(consolePath);
		const { solverState } = await import(solverPath);
		let operationId = 0;
		let operationStatus = '';
		const unsubscribe = consoleState.subscribe((state: ConsoleState) => {
			operationId = state.operationId ?? 0;
			operationStatus = state.operationStatus ?? '';
		});
		unsubscribe();
		solverState.update((state: Solver) => ({
			...state,
			operationId: 0,
			operationStatus: 'idle'
		}));
		return { operationId, operationStatus };
	});
	expect({ operationId, operationStatus }).toEqual({ operationId: 1, operationStatus: 'pending' });
	await page.setViewportSize({ width: 800, height: 900 });
	await page.getByRole('button', { name: 'Diagnostics' }).click();
	await expect(consoleBreak).toBeVisible();
	await expect(page.locator('.ui-panel[data-panel="solver"]')).toBeHidden();
});

test('table plot keeps a failed max-points draft until a successful retry', async ({ page }) => {
	let requests = 0;
	let accept!: () => void;
	const acceptance = new Promise<void>((resolve) => {
		accept = resolve;
	});
	const maxPointsValues: number[] = [];
	await page.route('**/api/tableplot/maxpoints', async (route) => {
		requests++;
		maxPointsValues.push(route.request().postDataJSON().maxPoints);
		if (requests === 1) {
			await route.fulfill({ status: 503, json: { error: 'temporary failure' } });
			return;
		}
		await acceptance;
		await route.fulfill({ status: 204 });
	});

	await page.goto('/');
	await page.evaluate(async () => {
		const websocketPath = '/src/api/websocket.ts';
		const tablePlotPath = '/src/api/incoming/table-plot.ts';
		const { connectionState } = await import(websocketPath);
		const { tablePlotState } = await import(tablePlotPath);
		connectionState.set('connected');
		tablePlotState.update((state: TablePlot) => ({
			...state,
			columns: ['t', 'mx'],
			data: [
				[0, 0],
				[1, 1]
			],
			xColumn: 't',
			yColumn: 'mx',
			maxPoints: 1000,
			step: 1
		}));
	});

	const input = page.getByLabel('Max points');
	await input.fill('42');
	await input.press('Tab');
	await expect.poll(() => requests).toBe(1);
	await expect(page.locator('.toast-stack[role="status"]')).toContainText('temporary failure');
	await expect(input).toHaveValue('42');

	await input.press('Enter');
	await expect.poll(() => requests).toBe(2);
	await expect(input).toBeDisabled();
	await expect(input).toHaveValue('42');
	await page.waitForTimeout(100);
	await expect.poll(() => requests).toBe(2);
	expect(maxPointsValues).toEqual([42, 42]);
	accept();
	await expect(input).toHaveValue('');
});
