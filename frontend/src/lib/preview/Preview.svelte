<script lang="ts">
	import { connected,previewConnected,previewError,setPreviewVisible, setPreviewClientBudget } from '$api/websocket';
	import { meshState } from '$api/incoming/mesh';
	import { previewState } from '$api/incoming/preview';
	import {
		postAllLayers,postMaxPoints,postScale,
		postComponent,
		postAutoScaleEnabled,
		postLayer,
		postQuantity,
		postXChosenSize,
		postZChosenSize,
		postYChosenSize
	} from '$api/outgoing/preview';
	import Slider from '$components/Slider.svelte';
	import Button from '$lib/ui/Button.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import Panel from '$lib/ui/Panel.svelte';
	import SelectField from '$lib/ui/SelectField.svelte';
	import SegmentedControl from '$lib/ui/SegmentedControl.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import Toggle from '$lib/ui/Toggle.svelte';
	import { panelPreferences, setPreferredPreviewMode } from '$lib/ui/preferences';
	import type { SelectOption } from '$lib/ui/SelectField.svelte';
	import type { ViewportMode } from '$lib/ui/types';
	import { get } from 'svelte/store';
	import { onDestroy, onMount } from 'svelte';
	import { preview2D, resizeECharts,disposePreview2D } from './preview2D';
	import {
		preview3D,
		qualityLevel,
		renderMode,
		resetCamera,
		setQuality,
		setRenderMode,
		threeDPreview,
		visibleRenderCount,previewPerformance,disposePreview3D,
		type Preview3DRenderMode,
		type QualityLevel
	} from './preview3D';
	import Toolbar3D from './inputs/Toolbar3D.svelte';
	import ViewCube from './ViewCube.svelte';
	import { quantities } from './inputs/quantities';

	let previewNow=$state(Date.now());
 let viewMode = $state<ViewportMode>('inline');
	let previewWrapper: HTMLDivElement;
 let visibilityObserver:IntersectionObserver|null=null;

	let popX = $state(60);
	let popY = $state(60);
	let popW = $state(760);
	let popH = $state(540);
	let dragging = $state(false);
	let resizing = $state(false);
	let dragOffX = $state(0);
	let dragOffY = $state(0);

	const quantityOptions = $derived(
		Object.entries(quantities).flatMap(([group, items]) =>
			items.map(
				(item) =>
					({
						value: item,
						label: item,
						group: group === 'Common' ? undefined : group
					}) satisfies SelectOption
			)
		)
	);

	const componentOptions = $derived([
		{ value: '3D', label: '3D', disabled: $previewState.nComp === 1 },
		{ value: 'x', label: 'x', disabled: $previewState.nComp === 1 },
		{ value: 'y', label: 'y', disabled: $previewState.nComp === 1 },
		{ value: 'z', label: 'z', disabled: $previewState.nComp === 1 }
	]);

	const qualityOptions = $derived(
		(['low', 'high', 'ultra'] as QualityLevel[]).map((level) => ({
			value: level,
			label: level.toUpperCase()
		}))
	);

	const zSamplingSupported = $derived(($previewState.zPossibleSizes?.length ?? 0) > 0);
	const zSizes = $derived(zSamplingSupported ? $previewState.zPossibleSizes :
		Array.from({ length: Math.max($meshState.Nz, 1) }, (_, i) => i + 1)
			.filter(n => $meshState.Nz % n === 0));

	const renderOptions = $derived(
		(['glyph', 'voxel'] as Preview3DRenderMode[]).map((mode) => ({
			value: mode,
			label: mode === 'glyph' ? 'Arrows' : 'Voxel'
		}))
	);

	const hasData = $derived(
		($previewState.scalarField?.length ?? 0) > 0 ||
			($previewState.vectorFieldPositions?.length ?? 0) > 0
	);
	const previewTone = $derived(
		!$connected ? 'warn' : $previewState.autoDownscaled ? 'warn' : hasData ? 'info' : 'default'
	);

	$effect(() => {
		viewMode = $panelPreferences.preferredPreviewMode;
	});

	function setMode(mode: ViewportMode) {
		if (viewMode === 'fullscreen' && document.fullscreenElement) {
			document.exitFullscreen().catch(() => undefined);
		}

		if (mode === 'fullscreen' && previewWrapper) {
			previewWrapper.requestFullscreen().catch(() => undefined);
		}

		viewMode = mode;
		setPreferredPreviewMode(mode);
		scheduleResize();
	}

	function scheduleResize() {
		window.setTimeout(() => {
			const container = document.getElementById('container');
			const display = get(threeDPreview);
			if (display && container) {
				display.renderer.setSize(container.clientWidth, container.clientHeight);
				display.camera.aspect = container.clientWidth / container.clientHeight;
				display.camera.updateProjectionMatrix();
			}
			resizeECharts();
		}, 120);
	}


	function onFullscreenChange() {
		if (!document.fullscreenElement && viewMode === 'fullscreen') {
			viewMode = 'inline';
			setPreferredPreviewMode('inline');
		}
		scheduleResize();
	}

	function startDrag(event: MouseEvent) {
		if (resizing) {
			return;
		}
		dragging = true;
		dragOffX = event.clientX - popX;
		dragOffY = event.clientY - popY;
		event.preventDefault();
	}

	function startResize(event: MouseEvent) {
		resizing = true;
		dragOffX = event.clientX;
		dragOffY = event.clientY;
		event.preventDefault();
	}

	function onMouseMove(event: MouseEvent) {
		if (dragging) {
			popX = event.clientX - dragOffX;
			popY = event.clientY - dragOffY;
		} else if (resizing) {
			const dx = event.clientX - dragOffX;
			const dy = event.clientY - dragOffY;
			popW = Math.max(460, popW + dx);
			popH = Math.max(340, popH + dy);
			dragOffX = event.clientX;
			dragOffY = event.clientY;
		}
	}

	function onMouseUp() {
		if (dragging || resizing) {
			dragging = false;
			resizing = false;
			scheduleResize();
		}
	}

	function onPreviewModeRequest(event: Event) {
		const customEvent = event as CustomEvent<ViewportMode>;
		setMode(customEvent.detail);
	}

	async function renderCurrentPreview() {
		const state = get(previewState);
		if (state.type === '3D') {
			await preview3D();
			return;
		}

		await preview2D();
	}

	onMount(() => {
 const ageTimer=setInterval(()=>{previewNow=Date.now()},1000);
 visibilityObserver=new IntersectionObserver(entries=>setPreviewVisible(entries[0]?.isIntersecting??true));
 if(previewWrapper)visibilityObserver.observe(previewWrapper);
		document.addEventListener('fullscreenchange', onFullscreenChange);
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		window.addEventListener('amumax:preview-mode', onPreviewModeRequest as EventListener);
		scheduleResize();
		void renderCurrentPreview();
 return ()=>clearInterval(ageTimer);
	});

	onDestroy(() => {
 visibilityObserver?.disconnect();setPreviewVisible(false);disposePreview3D();disposePreview2D();
		document.removeEventListener('fullscreenchange', onFullscreenChange);
		document.removeEventListener('mousemove', onMouseMove);
		document.removeEventListener('mouseup', onMouseUp);
		window.removeEventListener('amumax:preview-mode', onPreviewModeRequest as EventListener);
	});
</script>

<Panel
	title="Preview"
	subtitle="Primary simulation surface with consistent controls for 2D and 3D modes."
	panelId="preview"
	eyebrow="Visualization"
	tone={previewTone}
>
	{#snippet actions()}
		<StatusBadge
			label={$previewState.type || 'Awaiting data'}
			tone={hasData ? 'info' : 'default'}
		/>
		<div class="preview-mode-switcher">
			<Button
				size="sm"
				variant={viewMode === 'inline' ? 'solid' : 'outline'}
				tone="info"
				onclick={() => setMode('inline')}
			>
				Dock
			</Button>
			<Button
				size="sm"
				variant={viewMode === 'popout' ? 'solid' : 'outline'}
				tone="info"
				onclick={() => setMode('popout')}
			>
				Popout
			</Button>
			<Button
				size="sm"
				variant={viewMode === 'fullscreen' ? 'solid' : 'outline'}
				tone="info"
				onclick={() => setMode(viewMode === 'fullscreen' ? 'inline' : 'fullscreen')}
			>
				{viewMode === 'fullscreen' ? 'Exit' : 'Fullscreen'}
			</Button>
		</div>
	{/snippet}

	<div class="preview-toolbar">
		<div class="preview-controls-row preview-controls-row--data">
			<SelectField label="Quantity" value={$previewState.quantity} options={quantityOptions} onchange={postQuantity} />
			<SegmentedControl label="Component" value={$previewState.component} options={componentOptions} onchange={postComponent} />
		</div>

		<fieldset class="preview-controls-group">
			<legend>Layers &amp; resolution</legend>
			<div class="preview-controls-row preview-controls-row--mode">
				<SegmentedControl label="Show" value={$previewState.allLayers ? 'all' : 'single'}
					options={[
						{ value: 'single', label: 'Single layer' },
						{ value: 'all', label: 'All layers', disabled: $meshState.Nz < 2 }
					]}
					onchange={(mode) => postAllLayers(mode === 'all')} />
				<Toggle label="Auto-adjust resolution" checked={$previewState.autoScaleEnabled} onchange={postAutoScaleEnabled} />
			</div>
			<div class="preview-resolution-grid">
				{#if $previewState.xPossibleSizes.length > 0}
					<Slider label="X data points" value={$previewState.xChosenSize} values={$previewState.xPossibleSizes} onChangeFunction={postXChosenSize} />
				{/if}
				{#if $previewState.yPossibleSizes.length > 0}
					<Slider label="Y data points" value={$previewState.yChosenSize} values={$previewState.yPossibleSizes} onChangeFunction={postYChosenSize} />
				{/if}
				{#if $previewState.allLayers && $previewState.type === '3D'}
					<Slider label="Z data points" value={$previewState.zChosenSize || $meshState.Nz}
						values={zSizes} onChangeFunction={postZChosenSize} isDisabled={!zSamplingSupported} />
				{:else if !$previewState.allLayers && $meshState.Nz > 1}
					<Slider label="Z layer" value={$previewState.layer}
						values={Array.from({ length: $meshState.Nz }, (_, i) => i)} onChangeFunction={postLayer} />
				{/if}
			</div>
			{#if $previewState.allLayers && $previewState.type === '3D' && !zSamplingSupported}
				<p class="preview-control-hint" role="status">Z resolution is unavailable in this running simulation. Start it with an updated application to enable this control.</p>
			{:else if $previewState.allLayers && $previewState.type !== '3D'}
				<p class="preview-control-hint">All layers are projected onto the XY plane. Select the 3D component to adjust Z resolution.</p>
			{/if}
		</fieldset>

		<fieldset class="preview-controls-group">
			<legend>Appearance</legend>
   <label>Preview budget <select aria-label="Preview budget" value={$previewState.maxPoints} onchange={(e)=>postMaxPoints(Number(e.currentTarget.value))}>
   {#each [131072,262144,500000,1000000] as budget}<option value={budget}>{budget.toLocaleString()} samples</option>{/each}
   </select></label>
 <label>Client sampling <select aria-label="Client sampling budget" onchange={(e)=>setPreviewClientBudget(Number(e.currentTarget.value))}><option value={131072}>128k</option><option value={262144} selected>262k</option><option value={1000000}>1M</option></select></label>
   <label>Field scale (0 = adaptive) <input aria-label="Field scale" type="number" min="0" step="any" value={$previewState.fixedScale??0} onchange={(e)=>postScale(Number(e.currentTarget.value))} /></label>
			<div class="preview-controls-row preview-controls-row--appearance">
				{#if $previewState.type === '3D' && $previewState.nComp === 3}
					<SegmentedControl label="Render" value={$renderMode} options={renderOptions} onchange={(next) => setRenderMode(next as Preview3DRenderMode)} />
					<SegmentedControl label="Quality" value={$qualityLevel} options={qualityOptions} onchange={(next) => setQuality(next as QualityLevel)} />
				{/if}
				<Button variant="outline" tone="accent" onclick={resetCamera} disabled={$previewState.nComp !== 3 || $previewState.type !== '3D'}>Reset camera</Button>
			</div>
		</fieldset>
	</div>

	{#if $previewError}<p role="alert">{$previewError}</p>{/if}
 {#if $connected&&!$previewConnected}<p class="preview-control-hint">Preview stream disconnected; the displayed field may be stale.</p>{/if}
 <p class="preview-control-hint">{#if $previewState.allLayers&&$previewState.type!=='3D'}Signed max-abs projection over every Z layer; XY values are area averages.{:else}XY values are area averages; Z uses sampled layers. Sampling and thresholds can hide thin or opposing structures.{/if}</p>

 {#if $previewState.autoDownscaled && $previewState.autoDownscaleMessage}
		<div class="preview-notice">
			<StatusBadge label="Auto-scaled" tone="warn" />
			<p>{$previewState.autoDownscaleMessage}</p>
		</div>
	{/if}

	<div
		class="preview-wrapper"
		class:preview-wrapper--popout={viewMode === 'popout'}
		class:preview-wrapper--fullscreen={viewMode === 'fullscreen'}
		style={viewMode === 'popout'
			? `left:${popX}px;top:${popY}px;width:${popW}px;height:${popH}px;`
			: ''}
		bind:this={previewWrapper}
	>
		{#if viewMode === 'popout'}
			<div class="preview-wrapper__titlebar" role="presentation" onmousedown={startDrag}>
				<span>Floating preview</span>
				<div class="preview-wrapper__title-actions">
					<Button size="sm" variant="ghost" tone="info" onclick={() => setMode('inline')}
						>Dock</Button
					>
					<Button size="sm" variant="ghost" tone="info" onclick={() => setMode('fullscreen')}
						>Fullscreen</Button
					>
				</div>
			</div>
		{/if}

		<Toolbar3D />
		<ViewCube />

		{#if !$connected || !hasData}
			<div class="preview-wrapper__empty">
				<EmptyState
					title={!$connected ? 'Preview offline' : 'No preview data yet'}
					description={!$connected
						? 'Reconnect to the backend to stream scalar or vector fields.'
						: 'This surface will populate once the engine publishes preview data.'}
					tone={!$connected ? 'warn' : 'info'}
				/>
			</div>
		{/if}

		<div id="container" class="preview-wrapper__canvas"></div>

		{#if $previewState.type === '3D' && $previewState.nComp === 3 && hasData}
			<div class="preview-wrapper__stats">
				{$renderMode === 'voxel' ? 'Voxels' : 'Arrows'}: {$visibleRenderCount.toLocaleString()} / {$previewState.vectorCount.toLocaleString()}
    · {$previewPerformance.updateMs.toFixed(1)} ms update
 {#if $previewState.timestamp} · step {$previewState.step} · {Math.max(0,(previewNow-$previewState.timestamp)/1000).toFixed(1)} s old{/if}
    {#if $previewPerformance.lod} · simplified geometry{/if}
    {#if $previewState.normScale} · sampling {$previewState.transportSampling||1}× · scale {$previewState.normScale.toPrecision(3)}{/if}
    {#if $previewState.invalidCount} · invalid {$previewState.invalidCount}{/if}
			</div>
		{/if}

		{#if viewMode === 'popout'}
			<div class="preview-wrapper__resize" role="presentation" onmousedown={startResize}></div>
		{/if}
	</div>
</Panel>

<style>
	.preview-mode-switcher {
		display: flex;
		gap: 0.45rem;
		flex-wrap: wrap;
	}

	.preview-toolbar {
		display: grid;
		gap: 0.85rem;
		container-type: inline-size;
	}

	.preview-controls-row, .preview-resolution-grid {
		display: grid;
		gap: 0.8rem;
		align-items: end;
		min-width: 0;
	}

	.preview-controls-row--data, .preview-controls-row--mode {
		grid-template-columns: repeat(2, minmax(0, 1fr));
	}

	.preview-controls-row--mode {
		align-items: center;
		margin-bottom: 0.85rem;
	}

	.preview-resolution-grid {
		grid-template-columns: repeat(3, minmax(0, 1fr));
	}

	.preview-controls-row--appearance {
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
	}

	.preview-controls-group {
		min-width: 0;
		margin: 0;
		padding: 0.8rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-md);
	}

	.preview-controls-group legend {
		padding: 0 0.4rem;
		font-size: 0.75rem;
		color: var(--text-3);
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}

	.preview-control-hint {
		margin: 0.8rem 0 0;
		font-size: 0.85rem;
		color: var(--text-2);
	}

	@container (max-width: 620px) {
		.preview-controls-row--appearance, .preview-controls-row--mode {
			grid-template-columns: 1fr;
		}
	}

	@container (max-width: 420px) {
		.preview-controls-row--data, .preview-resolution-grid {
			grid-template-columns: 1fr;
		}
	}

	.preview-notice {
		display: flex;
		align-items: flex-start;
		gap: 0.8rem;
		padding: 0.8rem 0.95rem;
		border-radius: var(--radius-md);
		border: 1px solid rgba(242, 180, 90, 0.28);
		background: rgba(242, 180, 90, 0.08);
	}

	.preview-notice p {
		margin: 0;
		font-size: 0.88rem;
		color: var(--text-2);
	}

	.preview-wrapper {
		position: relative;
		min-height: var(--canvas-min-height);
		border-radius: var(--radius-lg);
		border: 1px solid var(--border-subtle);
		background: linear-gradient(180deg, rgba(6, 10, 18, 0.98), rgba(7, 10, 17, 0.98)), #050811;
		overflow: hidden;
	}

	.preview-wrapper__canvas {
		width: 100%;
		height: var(--canvas-min-height);
	}

	.preview-wrapper--popout {
		position: fixed;
		z-index: var(--z-popout);
		box-shadow: var(--shadow-panel);
		display: flex;
		flex-direction: column;
	}

	.preview-wrapper--popout .preview-wrapper__canvas {
		flex: 1;
		height: 100%;
	}

	.preview-wrapper--fullscreen {
		background: #04070e;
	}

	.preview-wrapper--fullscreen .preview-wrapper__canvas {
		height: 100vh;
	}

	.preview-wrapper__titlebar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.65rem 0.75rem;
		border-bottom: 1px solid var(--border-subtle);
		background: rgba(9, 14, 25, 0.9);
		cursor: move;
	}

	.preview-wrapper__titlebar span {
		font-size: 0.86rem;
		color: var(--text-2);
	}

	.preview-wrapper__title-actions {
		display: flex;
		gap: 0.35rem;
	}

	.preview-wrapper__empty {
		position: absolute;
		inset: 1rem;
		z-index: 2;
		display: grid;
		place-items: center;
		pointer-events: none;
	}

	.preview-wrapper__empty :global(.ui-empty) {
		pointer-events: auto;
		width: min(28rem, 100%);
		backdrop-filter: blur(12px);
	}

	.preview-wrapper__stats {
		position: absolute;
		left: 0.9rem;
		bottom: 0.9rem;
		z-index: 2;
		padding: 0.35rem 0.6rem;
		border-radius: 999px;
		border: 1px solid rgba(255, 255, 255, 0.12);
		background: rgba(9, 14, 25, 0.84);
		backdrop-filter: blur(10px);
		color: var(--text-2);
		font-size: 0.78rem;
		font-family: 'IBM Plex Mono', monospace;
		pointer-events: none;
	}

	.preview-wrapper__resize {
		position: absolute;
		right: 0;
		bottom: 0;
		width: 1.2rem;
		height: 1.2rem;
		cursor: nwse-resize;
		background: linear-gradient(135deg, transparent 45%, rgba(107, 167, 255, 0.45) 45%);
	}

</style>
