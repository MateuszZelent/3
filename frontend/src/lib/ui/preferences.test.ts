import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$app/env', () => ({ browser: true }));

function storage(value: unknown, failWrite = false) {
	const setItem = vi.fn(() => {
		if (failWrite) throw new DOMException('Full', 'QuotaExceededError');
	});
	vi.stubGlobal('window', {
		innerWidth: 768,
		localStorage: { getItem: () => JSON.stringify(value), setItem }
	});
	vi.stubGlobal('document', { body: { dataset: {} } });
	return setItem;
}
beforeEach(() => vi.resetModules());
afterEach(() => vi.unstubAllGlobals());

describe('workspace preferences', () => {
	it('keeps the application usable when persistence fails', async () => {
		storage(null, true);
		const preferences = await import('./preferences');
		expect(get(preferences.panelPreferences).activeCompactTab).toBe('visualization');
		expect(() => preferences.setActiveCompactTab('controls')).not.toThrow();
		expect(get(preferences.panelPreferences).activeCompactTab).toBe('controls');
	});
	it('validates stored enums, booleans and finite dimensions', async () => {
		storage({
			activeCompactTab: 'invalid',
			preferredPreviewMode: {},
			densityMode: 'unknown',
			diagnosticsHeight: 'NaN',
			collapsedByPanel: { console: 'false', preview: true }
		});
		const preferences = await import('./preferences');
		const value = get(preferences.panelPreferences);
		expect(value).toMatchObject({
			activeCompactTab: 'visualization',
			preferredPreviewMode: 'inline',
			densityMode: 'compact',
			diagnosticsHeight: 352
		});
		expect(value.collapsedByPanel.console).toBe(false);
		expect(value.collapsedByPanel.preview).toBe(true);
	});
	it('limits expensive panels to the selected compact tab and respects collapse', async () => {
		storage(null);
		const preferences = await import('./preferences');
		preferences.workspaceModeState.set('compact');
		preferences.setActiveCompactTab('diagnostics');
		expect(get(preferences.visiblePanels)).toMatchObject({
			console: true,
			metrics: true,
			preview: false,
			tableplot: false,
			solver: false
		});
		preferences.setCollapsed('console', true);
		expect(get(preferences.visiblePanels).console).toBe(false);
		preferences.workspaceModeState.set('wide');
		expect(get(preferences.visiblePanels).preview).toBe(true);
	});
});
