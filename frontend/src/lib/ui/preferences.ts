import { browser } from '$app/env';
import { derived, writable } from 'svelte/store';

import type {
	PanelId,
	PanelPreferences,
	CompactTab,
	DensityMode,
	ViewportMode,
	WorkspaceMode
} from './types';

const STORAGE_KEY = 'amumax:panel-prefs';

const panelIds: PanelId[] = [
	'preview',
	'tableplot',
	'solver',
	'parameters',
	'mesh',
	'console',
	'metrics'
];

function defaultCollapsed() {
	return Object.fromEntries(panelIds.map((panelId) => [panelId, false])) as Record<
		PanelId,
		boolean
	>;
}

const defaults: PanelPreferences = {
	collapsedByPanel: defaultCollapsed(),
	activeCompactTab: 'visualization',
	diagnosticsHeight: 352,
	preferredPreviewMode: 'inline',
	densityMode: 'compact'
};

function sanitize(input: unknown): PanelPreferences {
	const value = input && typeof input === 'object' ? (input as Record<string, unknown>) : {};
	const storedCollapsed =
		value.collapsedByPanel && typeof value.collapsedByPanel === 'object'
			? (value.collapsedByPanel as Record<string, unknown>)
			: {};
	const collapsedByPanel = defaultCollapsed();
	for (const id of panelIds) {
		if (typeof storedCollapsed[id] === 'boolean') collapsedByPanel[id] = storedCollapsed[id];
	}
	return {
		collapsedByPanel,
		activeCompactTab: ['visualization', 'controls', 'inspectors', 'diagnostics'].includes(
			String(value.activeCompactTab)
		)
			? (value.activeCompactTab as CompactTab)
			: defaults.activeCompactTab,
		diagnosticsHeight:
			typeof value.diagnosticsHeight === 'number' && Number.isFinite(value.diagnosticsHeight)
				? Math.max(280, Math.min(value.diagnosticsHeight, 640))
				: defaults.diagnosticsHeight,
		preferredPreviewMode: ['inline', 'popout', 'fullscreen'].includes(
			String(value.preferredPreviewMode)
		)
			? (value.preferredPreviewMode as ViewportMode)
			: defaults.preferredPreviewMode,
		densityMode:
			value.densityMode === 'compact' || value.densityMode === 'cozy'
				? value.densityMode
				: defaults.densityMode
	};
}

function loadPreferences(): PanelPreferences {
	if (!browser) {
		return defaults;
	}

	try {
		const raw = window.localStorage.getItem(STORAGE_KEY);
		if (!raw) {
			return defaults;
		}
		return sanitize(JSON.parse(raw));
	} catch {
		return defaults;
	}
}

export const panelPreferences = writable<PanelPreferences>(loadPreferences());
export const workspaceModeState = writable<WorkspaceMode>('wide');
const panelTabs: Record<PanelId, CompactTab> = {
	preview: 'visualization',
	tableplot: 'visualization',
	solver: 'controls',
	mesh: 'inspectors',
	parameters: 'inspectors',
	console: 'diagnostics',
	metrics: 'diagnostics'
};
export const visiblePanels = derived(
	[panelPreferences, workspaceModeState],
	([prefs, mode]) =>
		Object.fromEntries(
			panelIds.map((id) => [
				id,
				!prefs.collapsedByPanel[id] && (mode === 'wide' || panelTabs[id] === prefs.activeCompactTab)
			])
		) as Record<PanelId, boolean>
);

if (browser) {
	panelPreferences.subscribe((value) => {
		try {
			window.localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
		} catch {
			// In-memory preferences remain usable when storage is unavailable or full.
		}
		if (document.body) document.body.dataset.density = value.densityMode;
	});
}

export function setCollapsed(panelId: PanelId, collapsed: boolean) {
	panelPreferences.update((prefs) => ({
		...prefs,
		collapsedByPanel: {
			...prefs.collapsedByPanel,
			[panelId]: collapsed
		}
	}));
}

export function toggleCollapsed(panelId: PanelId) {
	panelPreferences.update((prefs) => ({
		...prefs,
		collapsedByPanel: {
			...prefs.collapsedByPanel,
			[panelId]: !prefs.collapsedByPanel[panelId]
		}
	}));
}

export function setActiveCompactTab(activeCompactTab: CompactTab) {
	panelPreferences.update((prefs) => ({ ...prefs, activeCompactTab }));
}

export function setPreferredPreviewMode(preferredPreviewMode: ViewportMode) {
	panelPreferences.update((prefs) => ({ ...prefs, preferredPreviewMode }));
}

export function setDensityMode(densityMode: DensityMode) {
	panelPreferences.update((prefs) => ({ ...prefs, densityMode }));
}

export function setDiagnosticsHeight(diagnosticsHeight: number) {
	if (!Number.isFinite(diagnosticsHeight)) return;
	panelPreferences.update((prefs) => ({
		...prefs,
		diagnosticsHeight: Math.max(280, Math.min(diagnosticsHeight, 640))
	}));
}
