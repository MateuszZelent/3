import { expect, test } from './fixtures';

const viewports = [
	{ width: 1600, height: 1200 },
	{ width: 1440, height: 1024 },
	{ width: 1280, height: 960 },
	{ width: 1024, height: 900 },
	{ width: 768, height: 1024 }
];

for (const viewport of viewports) {
	test(`workspace shell ${viewport.width}x${viewport.height}`, async ({ page }) => {
		await page.setViewportSize(viewport);
		await page.goto('/');
		await expect(page.getByText('Workspace')).toBeVisible();
		await expect(page.getByText('Field studio', { exact: true })).toBeVisible();
		await expect(page).toHaveScreenshot(`workspace-shell-${viewport.width}.png`, {
			fullPage: true,
			animations: 'disabled',
			// Symbol glyphs (chevrons, icons) come from system fallback fonts that
			// differ between Linux hosts by up to ~400 pixels; layout changes
			// affect far more.
			maxDiffPixels: 800
		});
	});
}
