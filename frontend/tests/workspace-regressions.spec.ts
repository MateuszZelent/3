import { expect, test } from './fixtures';

test('rejected preview checkbox and scale changes restore the server values', async ({ page }) => {
	await page.route('**/api/preview/autoScaleEnabled', (route) =>
		route.fulfill({ status: 503, json: { error: 'budget change rejected' } })
	);
	await page.route('**/api/preview/scale', (route) =>
		route.fulfill({ status: 503, json: { error: 'scale change rejected' } })
	);
	await page.goto('/');
	const checkbox = page.getByLabel('Fit resolution to budget', { exact: true });
	const before = await checkbox.isChecked();
	await page.locator('label.ui-toggle').filter({ hasText: 'Fit resolution to budget' }).click();
	await expect(page.locator('.toast-stack[role="status"]')).toContainText('budget change rejected');
	await expect(checkbox).toBeChecked({ checked: before });
	await page.getByText('Arrow display', { exact: true }).click();
	const scale = page.getByLabel('Arrow length scale', { exact: true });
	const original = await scale.inputValue();
	await scale.fill('42');
	await scale.press('Tab');
	await expect(page.locator('.toast-stack[role="status"]')).toContainText('scale change rejected');
	await expect(scale).toHaveValue(original);
});

test('selecting the current preview dimension does not start a transition or send a command', async ({
	page
}) => {
	await page.goto('/');
	const selected = page.getByRole('button', { name: '3D volume', exact: true });
	await expect(selected).toHaveAttribute('aria-pressed', 'true');
	await page.evaluate(() => {
		const observer = new MutationObserver((records) => {
			for (const record of records)
				for (const node of record.addedNodes) {
					if (
						node instanceof Element &&
						(node.matches('.preview-transition') || node.querySelector('.preview-transition'))
					)
						document.body.dataset.unexpectedTransition = 'true';
				}
		});
		observer.observe(document.body, { childList: true, subtree: true });
	});
	await selected.click();
	await page.evaluate(
		() =>
			new Promise<void>((resolve) =>
				requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
			)
	);
	await expect(page.locator('.preview-transition')).toHaveCount(0);
	await expect(page.locator('body')).not.toHaveAttribute('data-unexpected-transition');
	await expect(page.getByRole('button', { name: 'Retry transition', exact: true })).toHaveCount(0);
});

test.beforeEach(async ({ page, baseURL }) => {
	await page.routeWebSocket(/\/ws/, () => {});
	await page.route(`${baseURL}/api/**`, (route) =>
		route.fulfill({ json: { id: 0, active: false, logs: [] } })
	);
	await page.route('https://fonts.googleapis.com/**', (route) =>
		route.fulfill({ contentType: 'text/css', body: '' })
	);
});

test('Space on navigation does not send a solver command', async ({ page }) => {
	const commands: string[] = [];
	page.on('request', (request) => {
		if (request.url().includes('/api/solver/')) commands.push(request.url());
	});
	await page.setViewportSize({ width: 768, height: 1024 });
	await page.goto('/');
	const navigation = page.getByRole('button', { name: 'Controls', exact: true });
	await navigation.focus();
	await page.keyboard.press('Space');
	await expect(page.locator('.zone-solver')).toBeVisible();
	expect(commands).toEqual([]);
});

test('compact navigation displays only the selected panel group', async ({ page }) => {
	await page.setViewportSize({ width: 768, height: 1024 });
	await page.goto('/');
	await page.getByRole('button', { name: 'Controls', exact: true }).click();
	await expect(page.locator('.zone-solver')).toBeVisible();
	await expect(page.locator('.zone-viz')).toBeHidden();
	await expect(page.locator('.zone-table')).toBeHidden();
	await page.getByRole('button', { name: 'Diagnostics', exact: true }).click();
	await expect(page.locator('.zone-console')).toBeVisible();
	await expect(page.locator('.zone-metrics')).toBeVisible();
	await expect(page.locator('.zone-solver')).toBeHidden();
	await page.setViewportSize({ width: 1440, height: 1024 });
	await expect(page.locator('.zone-solver')).toBeVisible();
	await expect(page.locator('.zone-viz')).toBeVisible();
});

test('unavailable preference storage does not prevent startup', async ({ page }) => {
	await page.addInitScript(() => {
		Storage.prototype.setItem = () => {
			throw new DOMException('Full', 'QuotaExceededError');
		};
	});
	await page.goto('/');
	await expect(page.getByText('Workspace', { exact: true })).toBeVisible();
	await expect(page.getByText('Field studio', { exact: true })).toBeVisible();
});

test('denied storage reads still allow the preview to load', async ({ page }) => {
	await page.addInitScript(() => {
		Object.defineProperty(window, 'localStorage', {
			get() {
				throw new DOMException('Denied', 'SecurityError');
			}
		});
	});
	await page.goto('/');
	await expect(page.getByText('Workspace', { exact: true })).toBeVisible();
	await expect(page.getByText('Field studio', { exact: true })).toBeVisible();
});

test('rejected solver settings preserve authoritative values and reset the control', async ({
	page,
	baseURL
}) => {
	const observed: number[] = [];
	await page.route(`${baseURL}/api/solver/type`, (route) =>
		route.fulfill({ status: 503, json: { error: 'Solver unavailable' } })
	);
	await page.route(`${baseURL}/api/solver/fixdt`, async (route) => {
		observed.push(
			await page.evaluate(async () => {
				const path = '/src/api/incoming/solver.ts';
				const { solverState } = await import(path);
				let value = 0;
				solverState.subscribe((state: { fixdt: number }) => {
					value = state.fixdt;
				})();
				return value;
			})
		);
		await route.fulfill({ status: 503, json: { error: 'Solver unavailable' } });
	});
	await page.goto('/');
	await page.evaluate(async () => {
		const path = '/src/api/incoming/solver.ts';
		const { solverState } = await import(path);
		solverState.update((state: object) => ({ ...state, type: 'rk45', fixdt: 1e-12 }));
	});
	await page.getByLabel('Solver type', { exact: true }).selectOption('euler');
	await expect(page.getByLabel('Solver type', { exact: true })).toHaveValue('rk45');
	await page.getByLabel('Fixdt', { exact: true }).fill('2e-12');
	await page.getByLabel('Fixdt', { exact: true }).press('Tab');
	await expect(page.getByLabel('Fixdt', { exact: true })).toHaveValue('1e-12');
	expect(observed).toEqual([1e-12]);
});

test('accepted long solver operations disable duplicates while Break stays available', async ({
	page,
	baseURL
}) => {
	let runs = 0;
	let breaks = 0;
	await page.route(`${baseURL}/api/solver/run`, (route) => {
		runs++;
		return route.fulfill({ status: 202, json: { operationId: 42, status: 'pending' } });
	});
	await page.route(`${baseURL}/api/solver/break`, async (route) => {
		breaks++;
		await page.evaluate(async () => {
			const path = '/src/api/incoming/solver.ts';
			const { solverState } = await import(path);
			solverState.update((state: object) => ({
				...state,
				operationId: 42,
				operationStatus: 'cancelled'
			}));
		});
		await route.fulfill({ json: null });
	});
	await page.goto('/');
	await page.getByRole('button', { name: 'Run', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Run', exact: true })).toBeDisabled();
	await expect(page.getByRole('button', { name: 'Relax', exact: true })).toBeDisabled();
	const breakButtons = page.getByRole('button', { name: 'Break', exact: true });
	await expect(breakButtons).toHaveCount(2);
	await expect(breakButtons.nth(0)).toBeEnabled();
	await expect(breakButtons.nth(1)).toBeEnabled();
	await page.evaluate(() => {
		document.body.tabIndex = -1;
		document.body.focus();
	});
	await page.keyboard.press('Space');
	await expect(page.getByRole('button', { name: 'Run', exact: true })).toBeEnabled();
	expect(runs).toBe(1);
	expect(breaks).toBe(1);
});
