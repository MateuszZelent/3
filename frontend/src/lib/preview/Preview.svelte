<script lang="ts">
	import {
		previewTransition,
		previewTransitionError,
		requestPreviewTransition
	} from '$api/previewTransition';
	import {
		connected,
		previewConnected,
		previewError,
		setPreviewVisible,
		previewClientBudget,
		setPreviewClientBudget
	} from '$api/websocket';
	import { meshState } from '$api/incoming/mesh';
	import { previewState } from '$api/incoming/preview';
	import {
		postAllLayers,
		postSection,
		postPlaneResolution,
		postMaxPoints,
		postFullResolution,
		postScale,
		postComponent,
		postAutoScaleEnabled,
		postLayer,
		postXChosenSize,
		postZChosenSize,
		postYChosenSize
	} from '$api/outgoing/preview';
	import Slider from '$components/Slider.svelte';
	import SliceControl from './inputs/SliceControl.svelte';
	import VolumeControls from './inputs/VolumeControls.svelte';
	import RegionWindowControls from './inputs/RegionWindowControls.svelte';
	import PlaneWindowControls from './inputs/PlaneWindowControls.svelte';
	import {
		previewPlaneAxes,
		previewSampleCoordinateNm,
		type PreviewPlane
	} from './preview2DCoordinates';
	import Button from '$lib/ui/Button.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import Panel from '$lib/ui/Panel.svelte';
	import SelectField from '$lib/ui/SelectField.svelte';
	import SegmentedControl from '$lib/ui/SegmentedControl.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import Toggle from '$lib/ui/Toggle.svelte';
	import TextField from '$lib/ui/TextField.svelte';
	import { panelPreferences, setPreferredPreviewMode } from '$lib/ui/preferences';
	import type { SelectOption } from '$lib/ui/SelectField.svelte';
	import type { ViewportMode } from '$lib/ui/types';
	import { get } from 'svelte/store';
	import { onDestroy, onMount } from 'svelte';
	import { preview2D, resizeECharts, disposePreview2D, preview2DAutoscale } from './preview2D';
	import {
		preview3D,
		qualityLevel,
		renderMode,
		volumeProjection,
		volumeColorMode,
		resetCamera,
		setQuality,
		setRenderMode,
		threeDPreview,
		visibleRenderCount,
		previewPerformance,
		disposePreview3D,
		glyphSampling,
		voxelSampling,
		clipAxis,
		type Preview3DRenderMode,
		type QualityLevel
	} from './preview3D';
	import Toolbar3D from './inputs/Toolbar3D.svelte';
	import ViewCube from './ViewCube.svelte';
	import { quantities } from './inputs/quantities';

	let previewNow = $state(Date.now());
	let viewMode = $state<ViewportMode>('inline');
	let viewportWidth = $state(1024);
	let volumeControlsHeight = $state(0);
	let sectionExpanded = $state(true);
	$effect(() => {
		sectionExpanded = viewportWidth > 650;
	});
	let previewWrapper: HTMLDivElement;
	let previewStudio: HTMLDivElement;
	let visibilityObserver: IntersectionObserver | null = null;

	let popX = $state(60);
	let popY = $state(60);
	let popW = $state(760);
	let popH = $state(760);
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

	const is2D = $derived($previewState.type === '2D');
	const plane = $derived($previewState.plane || 'xy');
	const axes = $derived(previewPlaneAxes(plane));
	const axisCounts = $derived({ x: $meshState.Nx, y: $meshState.Ny, z: $meshState.Nz });
	const cellSizes = $derived({ x: $meshState.dx, y: $meshState.dy, z: $meshState.dz });
	const sliceCount = $derived(Math.max(axisCounts[axes.normal], 1));
	const sliceIndex = $derived($previewState.sliceIndex ?? $previewState.layer);
	const slicePosition = $derived(
		previewSampleCoordinateNm(sliceIndex, sliceCount, cellSizes[axes.normal] * sliceCount * 1e9)
	);
	const sectionCaption = $derived(
		is2D
			? $previewState.allLayers
				? `Mean through ${sliceCount} ${axes.normal.toUpperCase()} layers`
				: `${axes.normal} = ${Number.isFinite(slicePosition) ? Number(slicePosition.toPrecision(6)) : '—'} nm · layer ${sliceIndex}`
			: $previewState.allLayers
				? 'Full volume'
				: `Z layer ${$previewState.layer}`
	);
	const planeUSize = $derived($previewState.planeUChosenSize || $previewState.xChosenSize);
	const planeVSize = $derived($previewState.planeVChosenSize || $previewState.yChosenSize);
	const appliedU = $derived($previewState.appliedPlaneUSize || $previewState.appliedXChosenSize);
	const appliedV = $derived($previewState.appliedPlaneVSize || $previewState.appliedYChosenSize);
	let lastComponent = $state('x');
	$effect(() => {
		if (is2D && $previewState.nComp === 3 && ['x', 'y', 'z'].includes($previewState.component))
			lastComponent = $previewState.component;
	});
	const componentOptions = ['x', 'y', 'z'].map((value) => ({ value, label: value.toUpperCase() }));
	function setDimension(mode: string) {
		if (mode === '3D' && $previewState.nComp === 1) setRenderMode('volume');
		void requestPreviewTransition(
			'component',
			{ component: mode === '3D' ? '3D' : lastComponent },
			`Switching to ${mode === '3D' ? '3D volume' : '2D section'}`,
			{ type: mode }
		);
	}
	function changeQuantity(quantity: string) {
		void requestPreviewTransition('quantity', { quantity }, `Loading ${quantity}`, { quantity });
	}
	function setSlice(index: number) {
		if (Number.isInteger(index) && index >= 0 && index < sliceCount)
			void postSection({ sliceIndex: index });
	}

	const qualityOptions = $derived(
		(['low', 'high', 'ultra'] as QualityLevel[]).map((level) => ({
			value: level,
			label: level.toUpperCase()
		}))
	);

	const budgetOptions = [131072, 262144, 500000, 1000000].map((value) => ({
		value: String(value),
		label: `${value.toLocaleString()} points`
	}));
	const transferOptions = budgetOptions.map((option) => ({
		...option,
		label: option.value === '1000000' ? 'Full preview (up to 1M)' : option.label
	}));

	const zSamplingSupported = $derived(($previewState.zPossibleSizes?.length ?? 0) > 0);
	const zSizes = $derived(
		zSamplingSupported
			? $previewState.zPossibleSizes
			: Array.from({ length: Math.max($meshState.Nz, 1) }, (_, i) => i + 1).filter(
					(n) => $meshState.Nz % n === 0
				)
	);

	const renderOptions = $derived(
		(['volume', 'glyph', 'voxel'] as Preview3DRenderMode[]).map((mode) => ({
			value: mode,
			disabled: $previewState.nComp === 1 && mode !== 'volume',
			label: mode === 'volume' ? 'Volume' : mode === 'glyph' ? 'Arrows' : 'Voxel'
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

		if (mode === 'popout') {
			popW = Math.min(popW, window.innerWidth - 32);
			popH = Math.min(
				Math.max(popH, is2D || get(renderMode) === 'volume' ? 760 : 540),
				window.innerHeight - 32
			);
			popX = Math.max(16, Math.min(popX, window.innerWidth - popW - 16));
			popY = Math.max(16, Math.min(popY, window.innerHeight - popH - 16));
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
			popW = Math.max(Math.min(460, window.innerWidth - 32), popW + dx);
			popH = Math.max(
				Math.min(is2D || get(renderMode) === 'volume' ? 660 : 340, window.innerHeight - 32),
				popH + dy
			);
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
			if (state.nComp === 1 && get(renderMode) !== 'volume') setRenderMode('volume');
			await preview3D();
			return;
		}

		await preview2D();
	}

	onMount(() => {
		const ageTimer = setInterval(() => {
			previewNow = Date.now();
		}, 1000);
		const visibleSurfaces = new Map<Element, boolean>();
		visibilityObserver = new IntersectionObserver((entries) => {
			for (const entry of entries) visibleSurfaces.set(entry.target, entry.isIntersecting);
			setPreviewVisible([...visibleSurfaces.values()].some(Boolean));
		});
		if (previewStudio) visibilityObserver.observe(previewStudio);
		if (previewWrapper) visibilityObserver.observe(previewWrapper);
		document.addEventListener('fullscreenchange', onFullscreenChange);
		document.addEventListener('mousemove', onMouseMove);
		document.addEventListener('mouseup', onMouseUp);
		window.addEventListener('amumax:preview-mode', onPreviewModeRequest as EventListener);
		scheduleResize();
		void renderCurrentPreview();
		return () => clearInterval(ageTimer);
	});

	onDestroy(() => {
		visibilityObserver?.disconnect();
		setPreviewVisible(false);
		disposePreview3D();
		disposePreview2D();
		document.removeEventListener('fullscreenchange', onFullscreenChange);
		document.removeEventListener('mousemove', onMouseMove);
		document.removeEventListener('mouseup', onMouseUp);
		window.removeEventListener('amumax:preview-mode', onPreviewModeRequest as EventListener);
	});
</script>

<svelte:window bind:innerWidth={viewportWidth} />

<Panel
	title="Field studio"
	subtitle="Explore the volume. Inspect every plane."
	panelId="preview"
	eyebrow="Visualization"
	tone={previewTone}
>
	{#snippet actions()}
		<div class="preview-mode-switcher">
			<Button
				size="sm"
				variant={viewMode === 'inline' ? 'solid' : 'outline'}
				tone="info"
				onclick={() => setMode('inline')}>Dock</Button
			>
			<Button
				size="sm"
				variant={viewMode === 'popout' ? 'solid' : 'outline'}
				tone="info"
				onclick={() => setMode('popout')}>Popout</Button
			>
			<Button
				size="sm"
				variant={viewMode === 'fullscreen' ? 'solid' : 'outline'}
				tone="info"
				onclick={() => setMode(viewMode === 'fullscreen' ? 'inline' : 'fullscreen')}
				>{viewMode === 'fullscreen' ? 'Exit' : 'Fullscreen'}</Button
			>
		</div>
	{/snippet}

	<div class="preview-studio" bind:this={previewStudio}>
		<div class="studio-source">
			<SelectField
				label="Quantity"
				value={$previewTransition?.quantity || $previewState.quantity}
				disabled={!!$previewTransition}
				options={quantityOptions}
				onchange={changeQuantity}
			/>
			<SegmentedControl
				label="View"
				value={is2D ? '2D' : '3D'}
				options={[
					{ value: '3D', label: '3D volume', disabled: !!$previewTransition },
					{ value: '2D', label: '2D section', disabled: !!$previewTransition }
				]}
				onchange={setDimension}
			/>
			{#if is2D && $previewState.nComp === 3}<SegmentedControl
					label="Component"
					value={$previewState.component}
					options={componentOptions}
					onchange={postComponent}
				/>{:else}<div class="studio-source__description">
					<span>{$previewState.nComp === 1 ? 'Scalar field' : 'Vector field'}</span><strong
						>{$previewState.nComp === 1 ? 'Physical values' : 'Direction & magnitude'}</strong
					>
				</div>{/if}
		</div>

		{#if is2D}
			<div class="section-strip">
				<div class="plane-picker" role="group" aria-label="Section plane">
					{#each ['xy', 'yz', 'xz'] as value}
						<button
							type="button"
							class="plane-button"
							class:active={plane === value}
							aria-pressed={plane === value}
							aria-label={value.toUpperCase()}
							onclick={() => postSection({ plane: value as PreviewPlane })}
						>
							<svg viewBox="0 0 40 36" aria-hidden="true"
								><path
									class="plane-cube"
									d="M20 3 36 12 36 26 20 35 4 26 4 12ZM4 12 20 21 36 12M20 21V35M20 3V17"
								/><path
									class="plane-face"
									d={value === 'xy'
										? 'M4 12 20 3 36 12 20 21Z'
										: value === 'yz'
											? 'M4 12 20 21 20 35 4 26Z'
											: 'M20 21 36 12 36 26 20 35Z'}
								/></svg
							>
							<span
								>{value.toUpperCase()}<small
									>Cut {previewPlaneAxes(value as PreviewPlane).normal.toUpperCase()}</small
								></span
							>
						</button>
					{/each}
				</div>
				<SegmentedControl
					label="Reduction"
					value={$previewState.allLayers ? 'average' : 'single'}
					options={[
						{
							value: 'single',
							label: 'Single layer',
							disabled:
								$renderMode === 'volume' &&
								$volumeProjection === 'average' &&
								$volumeColorMode !== 'geometry'
						},
						{ value: 'average', label: 'Average' }
					]}
					onchange={(value) => postSection({ mode: value as 'single' | 'average' })}
				/>
			</div>
			{#if !$previewState.allLayers}<SliceControl
					axis={axes.normal}
					count={sliceCount}
					cellSize={cellSizes[axes.normal]}
					value={sliceIndex}
					onchange={setSlice}
				/>{:else}<p class="section-explanation">
					<span class="mean-symbol">μ</span>Arithmetic mean across all {sliceCount}
					{axes.normal.toUpperCase()} layers. Positive and negative values can cancel; zero-valued cells
					contribute to the mean.
				</p>{/if}
		{:else}
			<div class="volume-strip">
				<SegmentedControl
					label="Show"
					value={$previewState.allLayers ? 'all' : 'single'}
					options={[
						{ value: 'single', label: 'Single layer', disabled: $previewState.regionActive },
						{ value: 'all', label: 'All layers', disabled: $meshState.Nz < 2 }
					]}
					onchange={(mode) => postAllLayers(mode === 'all')}
				/>
				<SegmentedControl
					label="Render"
					value={$renderMode}
					options={renderOptions}
					onchange={(next) => setRenderMode(next as Preview3DRenderMode)}
				/>
				<Button variant="outline" tone="accent" onclick={resetCamera}>Reset camera</Button>
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

			<div class="surface-heading">
				<div class="surface-heading__field">
					<span class="surface-heading__dot" class:offline={!$previewConnected}></span><strong
						>{$previewState.quantity || 'Field'}{is2D && $previewState.nComp === 3
							? ` · ${$previewState.component}`
							: ''}</strong
					><span>{is2D ? plane.toUpperCase() : '3D'} / {sectionCaption}</span>
				</div>
				<span class="surface-heading__unit">{$previewState.unit || 'dimensionless'}</span
				>{#if viewMode === 'fullscreen'}<Button
						size="sm"
						variant="ghost"
						onclick={() => setMode('inline')}>Exit fullscreen</Button
					>{/if}
			</div>
			{#if viewMode !== 'inline' && is2D}
				<details class="floating-section" bind:open={sectionExpanded}>
					<summary>Section · {plane.toUpperCase()} · {sectionCaption}</summary>
					<div class="floating-section-controls">
						<SegmentedControl
							value={plane}
							options={['xy', 'yz', 'xz'].map((value) => ({ value, label: value.toUpperCase() }))}
							onchange={(value) => postSection({ plane: value as PreviewPlane })}
						/>
						<SegmentedControl
							value={$previewState.allLayers ? 'average' : 'single'}
							options={[
								{ value: 'single', label: 'Single layer' },
								{ value: 'average', label: 'Average' }
							]}
							onchange={(value) => postSection({ mode: value as 'single' | 'average' })}
						/>
						{#if !$previewState.allLayers}<SliceControl
								axis={axes.normal}
								count={sliceCount}
								cellSize={cellSizes[axes.normal]}
								value={sliceIndex}
								onchange={setSlice}
							/>{/if}
					</div>
				</details>
			{/if}
			{#if !is2D}<ViewCube axisBottom={volumeControlsHeight + 20} />{/if}

			{#if !$connected || !hasData}
				<div class="preview-wrapper__empty">
					<EmptyState
						title={!$connected
							? 'Preview offline'
							: is2D && $previewState.sequence
								? 'No cells in this section'
								: 'No preview data yet'}
						description={!$connected
							? 'Reconnect to the backend to stream scalar or vector fields.'
							: is2D && $previewState.sequence
								? $previewState.invalidCount
									? 'This section contains no finite values. Choose another layer or quantity.'
									: 'The selected cut does not intersect the geometry. Move the slice or choose Average.'
								: 'This surface will populate once the engine publishes preview data.'}
						tone={!$connected ? 'warn' : 'info'}
					/>
				</div>
			{/if}

			{#if is2D}<PlaneWindowControls />{/if}
			{#if $previewTransition}<div class="preview-transition" role="status" aria-live="polite">
					<span class="preview-transition__spinner" aria-hidden="true"></span>
					<div>
						<strong>{$previewTransition.label}</strong><span
							>{$previewTransition.phase === 'backend'
								? 'Waiting for the backend to prepare field data…'
								: 'Rendering the new view…'} · {Math.max(
								0,
								(previewNow - $previewTransition.startedAt) / 1000
							).toFixed(0)} s</span
						>
					</div>
				</div>{/if}
			<div id="container" class="preview-wrapper__canvas" aria-busy={!!$previewTransition}></div>
			{#if !is2D}<div bind:clientHeight={volumeControlsHeight}>
					{#if $previewState.region}<RegionWindowControls />{/if}
					<VolumeControls />
				</div>{/if}

			{#if $previewState.type === '3D' && hasData}
				<div class="preview-wrapper__stats">
					{$renderMode === 'volume'
						? 'Volume cells'
						: $renderMode === 'voxel'
							? 'Voxels'
							: 'Arrows'}: {$visibleRenderCount.toLocaleString()}
					· received {$previewState.vectorCount.toLocaleString()} / {(
						$previewState.serverVectorCount ?? $previewState.vectorCount
					).toLocaleString()} server points · grid {$previewState.appliedXChosenSize}
					× {$previewState.appliedYChosenSize} × {$previewState.allLayers
						? $previewState.appliedZChosenSize
						: 1}
					· display sampling {$renderMode === 'volume'
						? 1
						: $previewState.regionActive && $previewState.region?.mode === 'native'
							? 1
							: $renderMode === 'voxel'
								? $voxelSampling
								: $glyphSampling}×
					{#if $clipAxis !== 'none'}
						· section {$clipAxis.toUpperCase()}{/if}
					· {$previewPerformance.updateMs.toFixed(1)} ms update
					{#if $previewState.timestamp}
						· step {$previewState.step} · {Math.max(
							0,
							(previewNow - $previewState.timestamp) / 1000
						).toFixed(1)} s old{/if}
					{#if $previewPerformance.lod}
						· simplified geometry{/if}
					{#if $previewState.normScale}
						· transfer sampling {$previewState.transportSampling || 1}× · scale {$previewState.normScale.toPrecision(
							3
						)}{/if}
					{#if $previewState.invalidCount}
						· invalid {$previewState.invalidCount}{/if}
				</div>
			{:else if is2D && hasData}
				<div class="preview-wrapper__stats">
					<strong>{plane.toUpperCase()}</strong> · {$previewState.allLayers
						? 'Arithmetic mean'
						: 'Single layer'} · grid {appliedU} × {appliedV} · {$previewState.dataPointsCount.toLocaleString()}
					cells · {axes.u}/{axes.v} in nm{#if $previewState.timestamp}
						· step {$previewState.step} · {Math.max(
							0,
							(previewNow - $previewState.timestamp) / 1000
						).toFixed(1)} s old{/if}{#if $previewState.invalidCount}
						· invalid {$previewState.invalidCount}{/if}
				</div>
			{/if}

			{#if viewMode === 'popout'}
				<div class="preview-wrapper__resize" role="presentation" onmousedown={startResize}></div>
			{/if}
		</div>

		{#if $previewTransitionError}<p class="studio-alert" role="alert">
				{$previewTransitionError}
			</p>{/if}
		{#if $previewError}<p class="studio-alert" role="alert">{$previewError}</p>{/if}
		{#if $connected && !$previewConnected}<p class="studio-alert" role="status">
				Preview stream disconnected; the displayed field may be stale.
			</p>{/if}
		{#if $previewState.autoDownscaled && $previewState.autoDownscaleMessage}
			<div class="preview-notice">
				<StatusBadge
					label={$previewState.regionActive
						? $previewState.region?.mode === 'native'
							? 'Safety limit'
							: 'Window budget'
						: $previewState.autoScaleEnabled
							? 'Auto-scaled'
							: 'Safety limit'}
					tone="warn"
				/>
				<p>{$previewState.autoDownscaleMessage}</p>
			</div>
		{/if}

		<details class="studio-settings" open>
			<summary
				><span>Resolution &amp; rendering</span><span class="studio-settings__grid"
					>{is2D
						? `${appliedU} × ${appliedV}`
						: `${$previewState.appliedXChosenSize} × ${$previewState.appliedYChosenSize} × ${$previewState.allLayers ? $previewState.appliedZChosenSize : 1}`}<span
						class="studio-settings__chevron">⌄</span
					></span
				></summary
			>
			<div class="settings-grid">
				<section class="settings-card" aria-label="Preview resolution">
					{#if !is2D && $previewState.regionActive}
						<h3>Local window resolution</h3>
						<p class="settings-hint">
							Set Native cells or Custom resolution in Render window below the viewport. Full-domain
							resolution settings are retained.
						</p>
					{:else}
						<div class="settings-card__heading">
							<div>
								<span class="settings-card__eyebrow">Sampling</span>
								<h3>{is2D ? 'Plane resolution' : 'Volume resolution'}</h3>
							</div>
							<Button
								size="sm"
								variant="outline"
								onclick={postFullResolution}
								disabled={!$previewState.xPossibleSizes.length}>Full mesh resolution</Button
							>
						</div>
						<div class="resolution-sliders">
							{#if is2D}
								<Slider
									label={`${axes.u.toUpperCase()} data points`}
									value={planeUSize}
									values={$previewState.planeUPossibleSizes || $previewState.xPossibleSizes}
									onChangeFunction={(uSize) => postPlaneResolution({ uSize })}
								/>
								<Slider
									label={`${axes.v.toUpperCase()} data points`}
									value={planeVSize}
									values={$previewState.planeVPossibleSizes || $previewState.yPossibleSizes}
									onChangeFunction={(vSize) => postPlaneResolution({ vSize })}
								/>
							{:else}
								<Slider
									label="X data points"
									value={$previewState.xChosenSize}
									values={$previewState.xPossibleSizes}
									onChangeFunction={postXChosenSize}
								/>
								<Slider
									label="Y data points"
									value={$previewState.yChosenSize}
									values={$previewState.yPossibleSizes}
									onChangeFunction={postYChosenSize}
								/>
								{#if $previewState.allLayers}<Slider
										label="Z data points"
										value={$previewState.zChosenSize || $meshState.Nz}
										values={zSizes}
										onChangeFunction={postZChosenSize}
										isDisabled={!zSamplingSupported}
									/>{:else if $meshState.Nz > 1}<Slider
										label="Z layer"
										value={$previewState.layer}
										values={Array.from({ length: $meshState.Nz }, (_, i) => i)}
										onChangeFunction={postLayer}
									/>{/if}
							{/if}
						</div>
						<div class="resolution-summary">
							<span
								>Requested <strong
									>{is2D
										? `${planeUSize} × ${planeVSize}`
										: `${$previewState.xChosenSize} × ${$previewState.yChosenSize} × ${$previewState.allLayers ? $previewState.zChosenSize : 1}`}</strong
								></span
							><span
								>Applied <strong
									>{is2D
										? `${appliedU} × ${appliedV}`
										: `${$previewState.appliedXChosenSize} × ${$previewState.appliedYChosenSize} × ${$previewState.allLayers ? $previewState.appliedZChosenSize : 1}`}</strong
								></span
							>
						</div>
						<div class="budget-controls">
							<Toggle
								label="Auto-adjust resolution"
								checked={$previewState.autoScaleEnabled}
								onchange={postAutoScaleEnabled}
							/><SelectField
								label="Auto-adjust budget"
								value={$previewState.maxPoints}
								options={budgetOptions}
								disabled={!$previewState.autoScaleEnabled}
								onchange={(value) => postMaxPoints(Number(value))}
							/>
						</div>
						{#if !is2D && $previewState.allLayers && !zSamplingSupported}<p
								class="settings-hint"
								role="status"
							>
								Z resolution is unavailable in this running simulation. Restart it with the updated
								application to enable this control.
							</p>{/if}
						<p class="settings-hint">
							Area averages preserve coverage within each pixel.{#if !is2D}{' '}
								Z uses sampled layers.{/if} Budgets cap the requested grid.{#if !$previewState.autoScaleEnabled}{' '}
								Safety limit: 1,000,000 points.{/if}
						</p>
					{/if}
				</section>
				<section class="settings-card" aria-label="Preview appearance">
					<div class="settings-card__heading">
						<div>
							<span class="settings-card__eyebrow">Display</span>
							<h3>{is2D ? 'Calibrated heatmap' : '3D appearance'}</h3>
						</div>
						<StatusBadge
							label={is2D
								? $preview2DAutoscale
									? 'Autoscale'
									: 'Physical scale'
								: $renderMode === 'volume'
									? 'Volume'
									: $renderMode === 'voxel'
										? 'Voxel'
										: 'Arrows'}
							tone="info"
						/>
					</div>
					{#if is2D}
						<div class="heatmap-swatch" aria-hidden="true"></div>
						<div class="heatmap-features">
							<div>
								<strong>Flexible viewport</strong><span
									>Autoscale fills the window for long, narrow samples. Switch it off to preserve
									physical proportions. Axis coordinates remain in nanometres.</span
								>
							</div>
							<div>
								<strong>Signed values</strong><span
									>A diverging scale shows positive and negative fields. The legend follows the
									current section’s data range.</span
								>
							</div>
							<div>
								<strong>Inspect &amp; export</strong><span
									>Hover for cell coordinates and values. Use the chart toolbar to zoom, reset or
									save a PNG.</span
								>
							</div>
						</div>
					{:else}
						<SegmentedControl
							label="Quality"
							value={$qualityLevel}
							options={qualityOptions}
							onchange={(next) => setQuality(next as QualityLevel)}
						/>
						<div class="appearance-fields">
							<SelectField
								label="Transfer limit"
								value={$previewClientBudget}
								options={transferOptions.map((option) => ({
									...option,
									disabled: $renderMode === 'volume' && option.value !== '1000000'
								}))}
								onchange={(value) => setPreviewClientBudget(Number(value))}
							/><TextField
								label="Field scale"
								hint="0 = adaptive"
								type="number"
								min={0}
								step="any"
								value={$previewState.fixedScale ?? 0}
								onchange={(event) =>
									postScale(Number((event.currentTarget as HTMLInputElement).value))}
							/>
						</div>
						<p class="settings-hint">
							{#if $renderMode === 'volume'}Volume uses the full applied preview to preserve a
								continuous body. Adjust preview resolution to change geometry detail.{:else}The
								transfer limit can sample the server preview. Display sampling and clipping can
								further reduce visible points.{/if}
						</p>
						<details class="advanced-appearance">
							<summary>Lighting, clipping &amp; material</summary><Toolbar3D embedded />
						</details>
					{/if}
				</section>
			</div>
		</details>
	</div>
</Panel>

<style>
	.preview-transition {
		position: absolute;
		z-index: 8;
		top: 64px;
		left: 16px;
		right: 16px;
		display: flex;
		align-items: center;
		gap: 0.8rem;
		padding: 1rem;
		border: 1px solid var(--border-subtle);
		border-radius: 12px;
		background: var(--surface-1);
		box-shadow: 0 8px 24px #0005;
		pointer-events: none;
	}
	.preview-transition div {
		display: grid;
		gap: 0.3rem;
	}
	.preview-transition strong {
		font-size: 0.8rem;
	}
	.preview-transition span {
		font-size: 0.7rem;
		color: var(--text-2);
	}
	.preview-transition__spinner {
		width: 20px;
		height: 20px;
		flex: none;
		border: 2px solid var(--border-subtle);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: preview-spin 0.8s linear infinite;
	}
	@keyframes preview-spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.preview-transition__spinner {
			animation: none;
		}
	}

	.preview-studio {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		min-width: 0;
	}
	.preview-wrapper :global(.vc) {
		top: 4.2rem;
	}
	.preview-wrapper--popout :global(.vc) {
		top: 7rem;
	}
	.preview-wrapper :global(.ag) {
		bottom: 4.5rem;
	}
	:global([data-panel='preview'] > .ui-panel__body) {
		container-type: inline-size;
	}
	.preview-mode-switcher {
		display: flex;
		gap: 0.35rem;
		flex-wrap: wrap;
	}
	.studio-source {
		display: grid;
		grid-template-columns: minmax(0, 1.1fr) minmax(0, 1.2fr) minmax(0, 0.8fr);
		gap: 1rem;
		align-items: end;
	}
	.studio-source__description {
		display: grid;
		gap: 0.4rem;
		padding: 0.5rem 0.85rem;
		border-left: 1px solid var(--border-subtle);
	}
	.studio-source__description span,
	.settings-card__eyebrow {
		font-size: 0.69rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--text-3);
		font-weight: 600;
	}
	.studio-source__description strong {
		font-size: 0.8rem;
		color: var(--text-2);
		font-weight: 500;
	}
	.section-strip {
		display: grid;
		grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
		gap: 1.2rem;
		align-items: center;
	}
	.plane-picker {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.55rem;
	}
	.plane-button {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.7rem;
		min-width: 0;
		padding: 0.65rem 0.6rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: rgba(255, 255, 255, 0.025);
		color: var(--text-2);
		cursor: pointer;
		transition:
			background 0.16s,
			border-color 0.16s;
	}
	.plane-button:hover {
		background: rgba(87, 200, 182, 0.06);
		border-color: var(--border-interactive);
	}
	.plane-button.active {
		background: linear-gradient(135deg, rgba(87, 200, 182, 0.13), rgba(87, 200, 182, 0.035));
		border-color: rgba(87, 200, 182, 0.5);
		color: var(--accent);
		box-shadow: inset 0 1px rgba(87, 200, 182, 0.12);
	}
	.plane-button svg {
		width: 2.15rem;
		flex-shrink: 0;
		overflow: visible;
	}
	.plane-cube {
		fill: none;
		stroke: currentColor;
		opacity: 0.4;
		stroke-width: 1.2;
		stroke-linejoin: round;
	}
	.plane-face {
		fill: currentColor;
		fill-opacity: 0.3;
		stroke: currentColor;
		stroke-width: 1.2;
		stroke-linejoin: round;
	}
	.plane-button span {
		display: grid;
		gap: 0.15rem;
		font-size: 0.85rem;
		font-weight: 700;
		text-align: left;
	}
	.plane-button small {
		font-size: 0.65rem;
		font-weight: 400;
		color: var(--text-3);
	}
	.plane-button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	.section-explanation {
		display: flex;
		align-items: center;
		gap: 0.9rem;
		margin: 0;
		padding: 0.85rem 1rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-md);
		font-size: 0.8rem;
		line-height: 1.6;
		color: var(--text-2);
		background: rgba(87, 200, 182, 0.035);
	}
	.mean-symbol {
		color: var(--accent);
		font-size: 1.6rem;
		font-family: serif;
	}
	.volume-strip {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
		gap: 1rem;
		align-items: end;
	}
	.preview-wrapper {
		position: relative;
		display: flex;
		flex-direction: column;
		min-width: 0;
		border-radius: var(--radius-lg);
		border: 1px solid var(--border-subtle);
		background:
			radial-gradient(ellipse at 50% 10%, rgba(27, 43, 64, 0.3), transparent 75%), #070c14;
		overflow: hidden;
	}
	.surface-heading {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.75rem;
		padding: 0.8rem 1rem;
		border-bottom: 1px solid rgba(255, 255, 255, 0.05);
		background: rgba(255, 255, 255, 0.015);
	}
	.surface-heading__field {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
		font-size: 0.75rem;
		min-width: 0;
	}
	.surface-heading__field strong {
		font-family: var(--font-mono);
		font-size: 0.85rem;
		color: var(--text-1);
	}
	.surface-heading__field > span:last-child {
		color: var(--text-3);
	}
	.surface-heading__dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--accent);
		box-shadow: 0 0 8px rgba(87, 200, 182, 0.3);
	}
	.surface-heading__dot.offline {
		background: var(--text-3);
		box-shadow: none;
	}
	.surface-heading__unit {
		font-size: 0.7rem;
		color: var(--text-3);
		white-space: nowrap;
	}
	.preview-wrapper__canvas {
		width: 100%;
		height: clamp(25rem, 46vw, 39rem);
		min-height: 21rem;
		flex-shrink: 0;
	}
	.preview-wrapper__stats {
		padding: 0.6rem 1rem;
		border-top: 1px solid rgba(255, 255, 255, 0.055);
		background: rgba(255, 255, 255, 0.018);
		color: var(--text-3);
		font-size: 0.69rem;
		line-height: 1.8;
		font-family: var(--font-mono);
		pointer-events: none;
	}
	.preview-wrapper__stats strong {
		color: var(--text-2);
	}
	.preview-wrapper__empty {
		position: absolute;
		inset: 3.5rem 1rem 2rem;
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
	.studio-settings {
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: rgba(255, 255, 255, 0.015);
		min-width: 0;
	}
	.studio-settings > summary {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		align-items: center;
		list-style: none;
		padding: 0.9rem 1rem;
		cursor: pointer;
		font-weight: 600;
		font-size: 0.8rem;
		color: var(--text-2);
	}
	.studio-settings > summary::-webkit-details-marker {
		display: none;
	}
	.studio-settings__grid {
		display: flex;
		gap: 0.8rem;
		align-items: center;
		font-family: var(--font-mono);
		font-weight: 400;
		font-size: 0.75rem;
		color: var(--text-3);
	}
	.studio-settings__chevron {
		font-family: sans-serif;
		transition: transform 0.15s;
	}
	.studio-settings[open] .studio-settings__chevron {
		transform: rotate(180deg);
	}
	.settings-grid {
		display: grid;
		grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
		border-top: 1px solid var(--border-subtle);
	}
	.settings-card {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1.15rem;
		min-width: 0;
	}
	.settings-card + .settings-card {
		border-left: 1px solid var(--border-subtle);
	}
	.settings-card__heading {
		display: flex;
		justify-content: space-between;
		gap: 0.6rem;
		align-items: center;
	}
	.settings-card__heading h3 {
		font-size: 0.9rem;
		margin: 0.3rem 0 0;
		font-weight: 600;
		color: var(--text-1);
	}
	.resolution-sliders {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.8rem;
	}
	.resolution-sliders :global(.slider-field:nth-child(3)) {
		grid-column: 1/-1;
	}
	.resolution-summary {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		flex-wrap: wrap;
		font-size: 0.7rem;
		color: var(--text-3);
	}
	.resolution-summary strong {
		font-family: var(--font-mono);
		font-weight: 500;
		color: var(--text-2);
		margin-left: 0.4rem;
	}
	.budget-controls {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 0.9rem;
		align-items: center;
		padding-top: 0.9rem;
		border-top: 1px solid var(--border-subtle);
	}
	.settings-hint {
		margin: 0;
		font-size: 0.73rem;
		color: var(--text-2);
		line-height: 1.65;
	}
	.appearance-fields {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 0.8rem;
	}
	.heatmap-swatch {
		height: 0.5rem;
		border-radius: 999px;
		background: linear-gradient(90deg, #15315f, #90b9df, #f4f1ed, #efb09d, #7d1d34);
		opacity: 0.85;
	}
	.heatmap-features {
		display: grid;
		gap: 1rem;
	}
	.heatmap-features div {
		display: grid;
		gap: 0.3rem;
	}
	.heatmap-features strong {
		font-size: 0.77rem;
		font-weight: 600;
		color: var(--text-2);
	}
	.heatmap-features span {
		font-size: 0.74rem;
		line-height: 1.65;
		color: var(--text-2);
	}
	.advanced-appearance {
		border-top: 1px solid var(--border-subtle);
		padding-top: 0.75rem;
	}
	.advanced-appearance summary {
		font-size: 0.75rem;
		color: var(--text-2);
		cursor: pointer;
	}
	.studio-alert {
		margin: 0;
		font-size: 0.8rem;
		color: var(--warn);
	}
	.preview-notice {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		padding: 0.8rem 0.95rem;
		border-radius: var(--radius-md);
		border: 1px solid rgba(242, 180, 90, 0.28);
		background: rgba(242, 180, 90, 0.06);
	}
	.preview-notice p {
		margin: 0;
		font-size: 0.78rem;
		color: var(--text-2);
		line-height: 1.6;
	}
	.preview-wrapper--popout {
		position: fixed;
		z-index: var(--z-popout);
		box-shadow: var(--shadow-panel);
	}
	.preview-wrapper--popout .preview-wrapper__canvas,
	.preview-wrapper--fullscreen .preview-wrapper__canvas {
		flex: 1;
		min-height: 0;
		height: 100%;
	}
	.preview-wrapper--fullscreen {
		width: 100%;
		height: 100vh;
		background: #070c14;
	}
	.preview-wrapper__titlebar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.55rem 0.75rem;
		border-bottom: 1px solid var(--border-subtle);
		background: var(--surface-2);
		cursor: move;
	}
	.preview-wrapper__titlebar > span {
		font-size: 0.8rem;
		color: var(--text-2);
	}
	.preview-wrapper__title-actions {
		display: flex;
		gap: 0.35rem;
	}
	.preview-wrapper__resize {
		position: absolute;
		right: 0;
		bottom: 0;
		width: 1.2rem;
		height: 1.2rem;
		cursor: nwse-resize;
		background: linear-gradient(135deg, transparent 55%, rgba(107, 167, 255, 0.45) 55%);
	}
	.floating-section > summary {
		padding: 0.65rem 1rem;
		font-size: 0.75rem;
		color: var(--text-2);
		cursor: pointer;
		border-bottom: 1px solid var(--border-subtle);
	}
	.floating-section-controls {
		display: flex;
		gap: 0.75rem;
		flex-wrap: wrap;
		padding: 0.75rem 1rem;
		align-items: center;
	}
	.floating-section-controls :global(.slice-control) {
		flex: 1;
		min-width: 16rem;
	}
	@container (max-width: 800px) {
		.settings-grid {
			grid-template-columns: 1fr;
		}
		.settings-card + .settings-card {
			border-left: 0;
			border-top: 1px solid var(--border-subtle);
		}
	}
	@media (max-width: 650px) {
		:global([data-panel='preview'] > .ui-panel__header) {
			flex-wrap: wrap;
		}
	}
	@container (max-width: 650px) {
		.studio-source {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			gap: 0.8rem;
		}
		.studio-source > :last-child {
			grid-column: 1/-1;
		}
		.studio-source__description {
			display: none;
		}
		.section-strip {
			grid-template-columns: 1fr;
			gap: 0.85rem;
		}
		.volume-strip {
			grid-template-columns: minmax(0, 1fr);
		}
		.volume-strip :global(.ui-button) {
			grid-column: 1/-1;
		}
		.preview-wrapper__canvas {
			height: 25rem;
		}
		.surface-heading__unit {
			display: none;
		}
		.settings-card {
			padding: 1rem;
		}
		.budget-controls {
			grid-template-columns: 1fr;
		}
		.settings-card__heading {
			flex-wrap: wrap;
		}
		.studio-source :global(.ui-segmented__option) {
			padding: 0 0.55rem;
			font-size: 0.8rem;
		}
	}
</style>
