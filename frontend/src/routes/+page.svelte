<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import type { Component } from 'svelte';

	import { headerState } from '$api/incoming/header';
	import { connected, initializeWebSocket } from '$api/websocket';
	import { postBreak, postRun, solverOperationBusy } from '$api/outgoing/solver';
	import Alert from '$lib/alerts/Alert.svelte';
	import Console from '$lib/Console.svelte';
	import DemagProgress from '$lib/DemagProgress.svelte';
	import Header from '$lib/Header.svelte';
	import Mesh from '$lib/Mesh.svelte';
	import Metrics from '$lib/Metrics.svelte';
	import Parameters from '$lib/Parameters.svelte';
	import Solver from '$lib/Solver.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { panelPreferences, setActiveCompactTab, workspaceModeState } from '$lib/ui/preferences';
	import type { CompactTab, WorkspaceMode } from '$lib/ui/types';

	import '../app.css';

	let workspaceMode = $state<WorkspaceMode>('wide');
	let showShortcuts = $state(false);
	let PreviewComponent = $state<Component | null>(null);
	let TablePlotComponent = $state<Component | null>(null);
	let previewLoadError = $state('');
	let tablePlotLoadError = $state('');
	let disconnectWebSockets: (() => void) | undefined;

	const compactTabs: { id: CompactTab; label: string }[] = [
		{ id: 'visualization', label: 'Visuals' },
		{ id: 'controls', label: 'Controls' },
		{ id: 'inspectors', label: 'Inspectors' },
		{ id: 'diagnostics', label: 'Diagnostics' }
	];

	function syncWorkspaceMode() {
		workspaceMode =
			window.innerWidth < 768 ? 'mobile' : window.innerWidth < 1024 ? 'compact' : 'wide';
		workspaceModeState.set(workspaceMode);
	}

	async function requestPreviewMode(mode: 'inline' | 'popout' | 'fullscreen') {
		if (workspaceMode !== 'wide') {
			setActiveCompactTab('visualization');
			await tick();
		}
		window.dispatchEvent(new CustomEvent('amumax:preview-mode', { detail: mode }));
	}

	function isInteractiveTarget(target: EventTarget | null) {
		return (
			target instanceof Element &&
			!!target.closest(
				'input, textarea, select, button, a[href], summary, [contenteditable]:not([contenteditable="false"]), [role="button"], [role="slider"], [role="tab"], [role="textbox"], [role="checkbox"], [role="combobox"], [role="menuitem"], [role="dialog"], [aria-modal="true"]'
			)
		);
	}

	function handleShortcuts(event: KeyboardEvent) {
		if (event.defaultPrevented || event.repeat || event.metaKey || event.ctrlKey || event.altKey)
			return;
		if (showShortcuts) {
			if (event.key === 'Escape') {
				event.preventDefault();
				showShortcuts = false;
			}
			return;
		}

		if (isInteractiveTarget(event.target)) {
			return;
		}

		if (
			(event.key === '?' || (event.key === '/' && event.shiftKey)) &&
			!event.metaKey &&
			!event.ctrlKey
		) {
			event.preventDefault();
			showShortcuts = !showShortcuts;
			return;
		}

		if (!$connected) {
			return;
		}

		if (event.code === 'Space') {
			if (event.shiftKey) return;
			event.preventDefault();
			if ($headerState.status === 'running' || $solverOperationBusy) {
				postBreak();
			} else {
				postRun('1e-9');
			}
			return;
		}

		if (event.key === 'Escape') {
			postBreak();
			return;
		}

		if (event.key.toLowerCase() === 'f') {
			event.preventDefault();
			void requestPreviewMode('fullscreen');
		}
	}

	onMount(() => {
		disconnectWebSockets = initializeWebSocket();
		syncWorkspaceMode();
		window.addEventListener('resize', syncWorkspaceMode);
		window.addEventListener('keydown', handleShortcuts);

		void import('$lib/preview/Preview.svelte')
			.then((module) => {
				PreviewComponent = module.default;
			})
			.catch((error: unknown) => {
				previewLoadError = error instanceof Error ? error.message : String(error);
			});

		void import('$lib/table-plot/TablePlot.svelte')
			.then((module) => {
				TablePlotComponent = module.default;
			})
			.catch((error: unknown) => {
				tablePlotLoadError = error instanceof Error ? error.message : String(error);
			});
	});

	onDestroy(() => {
		disconnectWebSockets?.();
		window.removeEventListener('resize', syncWorkspaceMode);
		window.removeEventListener('keydown', handleShortcuts);
	});
</script>

<DemagProgress />

<Alert />

<div class="app-shell">
	<Header />

	{#if workspaceMode !== 'wide'}
		<div class="workspace-compact-tabs">
			{#each compactTabs as tab (tab.id)}
				<Button
					size="sm"
					variant={$panelPreferences.activeCompactTab === tab.id ? 'solid' : 'outline'}
					tone={$panelPreferences.activeCompactTab === tab.id ? 'accent' : 'info'}
					onclick={() => setActiveCompactTab(tab.id)}
				>
					{tab.label}
				</Button>
			{/each}
			<Button size="sm" variant="ghost" tone="info" onclick={() => (showShortcuts = true)}
				>Shortcuts</Button
			>
		</div>
	{/if}

	<div class="workspace">
		<div class="workspace-column">
			<div
				class="zone-viz"
				hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'visualization'}
			>
				{#if PreviewComponent}
					<PreviewComponent />
				{:else if previewLoadError}
					<div class="workspace-loading-card workspace-loading-card--error">
						Preview failed to load: {previewLoadError}
					</div>
				{:else}
					<div class="workspace-loading-card">Loading preview surface…</div>
				{/if}
			</div>
			<div
				class="zone-solver"
				hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'controls'}
			>
				<Solver />
			</div>
		</div>
		<div class="workspace-column">
			<div
				class="zone-table"
				hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'visualization'}
			>
				{#if TablePlotComponent}
					<TablePlotComponent />
				{:else if tablePlotLoadError}
					<div class="workspace-loading-card workspace-loading-card--error">
						Plots failed to load: {tablePlotLoadError}
					</div>
				{:else}
					<div class="workspace-loading-card">Loading analytical plot…</div>
				{/if}
			</div>
			<div
				class="zone-console"
				hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'diagnostics'}
			>
				<Console />
			</div>
			<div
				class="zone-mesh"
				hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'inspectors'}
			>
				<Mesh />
			</div>
			<div
				class="zone-params"
				hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'inspectors'}
			>
				<Parameters />
			</div>
		</div>

		<!-- Diagnostics spans both independent columns. -->
		<div
			class="zone-metrics"
			hidden={workspaceMode !== 'wide' && $panelPreferences.activeCompactTab !== 'diagnostics'}
		>
			<Metrics />
		</div>
	</div>

	{#if showShortcuts}
		<div class="shortcuts-overlay" role="dialog" aria-modal="true" aria-label="Keyboard shortcuts">
			<div class="shortcuts-card">
				<div class="shortcuts-card__header">
					<div>
						<p class="shortcuts-card__eyebrow">Keyboard</p>
						<h2>Shortcuts</h2>
					</div>
					<Button size="sm" variant="ghost" tone="info" onclick={() => (showShortcuts = false)}
						>Close</Button
					>
				</div>

				<div class="shortcuts-list">
					<div><kbd>Space</kbd><span>Run default runtime or break active run</span></div>
					<div><kbd>Esc</kbd><span>Break simulation or close this overlay</span></div>
					<div><kbd>F</kbd><span>Send Preview to fullscreen</span></div>
					<div><kbd>?</kbd><span>Toggle shortcuts overlay</span></div>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.shortcuts-overlay {
		position: fixed;
		inset: 0;
		z-index: var(--z-modal);
		display: grid;
		place-items: center;
		padding: 1rem;
		background: rgba(3, 7, 14, 0.72);
		backdrop-filter: blur(10px);
	}

	.shortcuts-card {
		width: min(32rem, 100%);
		display: grid;
		gap: 1rem;
		padding: 1.25rem;
		border-radius: calc(var(--radius-lg) + 0.1rem);
		border: 1px solid var(--border-subtle);
		background: linear-gradient(180deg, rgba(18, 28, 47, 0.98), rgba(8, 13, 23, 0.98));
		box-shadow: var(--shadow-panel);
	}

	.shortcuts-card__header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	.shortcuts-card__eyebrow {
		margin: 0 0 0.25rem;
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--text-3);
	}

	.shortcuts-card h2 {
		margin: 0;
		font-size: 1.2rem;
	}

	.shortcuts-list {
		display: grid;
		gap: 0.8rem;
	}

	.shortcuts-list div {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.8rem;
		align-items: center;
	}

	kbd {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 3.1rem;
		padding: 0.4rem 0.65rem;
		border-radius: var(--radius-pill);
		border: 1px solid var(--border);
		background: rgba(255, 255, 255, 0.03);
		font-family: 'IBM Plex Mono', monospace;
		color: var(--text-1);
	}

	.workspace-loading-card {
		display: grid;
		place-items: center;
		min-height: calc(var(--canvas-min-height) + 8.5rem);
		padding: 1rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background:
			linear-gradient(180deg, rgba(18, 28, 47, 0.96), rgba(9, 15, 25, 0.96)), var(--surface-1);
		color: var(--text-2);
		font-weight: 600;
		letter-spacing: 0.02em;
		box-shadow: var(--shadow-soft);
	}
</style>
