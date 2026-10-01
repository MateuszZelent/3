<script lang="ts">
	import { previewState } from '$api/incoming/preview';
	import SelectField from '$lib/ui/SelectField.svelte';
	import SegmentedControl from '$lib/ui/SegmentedControl.svelte';
	import Toggle from '$lib/ui/Toggle.svelte';
	import {
		volumeColorMode,
		voxelColorMode,
		renderMode,
		setVoxelColorMode,
		type VoxelColorMode,
		volumeProjection,
		volumeProjectionAxis,
		volumeLighting,
		volumeScaleMode,
		volumeManualRange,
		volumeLegend,
		setVolumeColorMode,
		setVolumeProjection,
		setVolumeLighting,
		setVolumeScale,
		type VolumeColorMode
	} from '../preview3D';
	import type { ProjectionAxis } from '../volumeProjection';
	const isVolume = $derived($renderMode === 'volume');
	const colorMode = $derived(isVolume ? $volumeColorMode : $voxelColorMode);
	function setColorMode(value: string) {
		if (isVolume) setVolumeColorMode(value as VolumeColorMode);
		else setVoxelColorMode(value as VoxelColorMode);
	}
	$effect(() => {
		if (
			$previewState.nComp === 1 &&
			colorMode !== 'geometry' &&
			colorMode !== 'value'
		)
			setColorMode('value');
		if ($previewState.nComp === 3 && colorMode === 'value') setColorMode('x');
	});
	let viewportWidth = $state(1024);
	let expanded = $state(true);
	$effect(() => {
		expanded = !isVolume || viewportWidth > 650;
	});
	const field = $derived(colorMode !== 'geometry');
	const scalar = $derived(field && colorMode !== 'orientation');
	const gradient = $derived(`linear-gradient(90deg, ${$volumeLegend.palette.join(',')})`);
	let min = $state('-1'),
		max = $state('1');
	const valid = $derived(
		Number.isFinite(Number(min)) && Number.isFinite(Number(max)) && Number(min) < Number(max)
	);
	function updateRange() {
		if (valid) setVolumeScale('manual', Number(min), Number(max));
	}
</script>

<svelte:window bind:innerWidth={viewportWidth} />
<details class="volume-controls" bind:open={expanded}>
	<summary class="volume-controls__heading">
		<span class="volume-controls__icon" aria-hidden="true">▧</span>
		<div>
			<strong>{isVolume ? "Volume surface" : $renderMode === "glyph" ? "Arrow colors" : "Voxel colors"}</strong><span
				>Field coloring · {$previewState.quantity || 'Field'}</span
			>
		</div>
		<span class="volume-controls__badge"
			>{isVolume && $volumeProjection === 'average' && field
				? `Mean · ${$volumeProjectionAxis.toUpperCase()}`
				: isVolume ? 'Surface' : 'Local'}</span
		>
	</summary>
	<div class="volume-controls__grid">
		<SelectField
			label="Color by"
			value={colorMode}
			options={$previewState.nComp === 1
				? [
						...(isVolume ? [{ value: 'geometry', label: 'Solid · geometry' }] : []),
						{ value: 'value', label: 'Field value' }
					]
				: [
						...(isVolume ? [{ value: 'geometry', label: 'Solid · geometry' }] : []),
						{ value: 'magnitude', label: 'Vector magnitude' },
						{ value: 'x', label: 'X component' },
						{ value: 'y', label: 'Y component' },
						{ value: 'z', label: 'Z component' },
						{ value: 'orientation', label: 'Vector orientation' }
					]}
			onchange={setColorMode}
		/>
		{#if isVolume}<SegmentedControl
			label="Field on surface"
			value={$volumeProjection}
			options={[
				{ value: 'surface', label: 'Surface values' },
				{ value: 'average', label: 'Thickness average' }
			]}
			onchange={(value) => setVolumeProjection(value as 'surface' | 'average')}
		/>
		{#if isVolume && $volumeProjection === 'average'}
			<SegmentedControl
				label="Average along"
				value={$volumeProjectionAxis}
				options={['x', 'y', 'z'].map((value) => ({ value, label: value.toUpperCase() }))}
				onchange={(value) => setVolumeProjection('average', value as ProjectionAxis)}
			/>
		{/if}
		{/if}
		{#if scalar}<SegmentedControl
				label="Color range"
				value={$volumeScaleMode}
				options={[
					{ value: 'data', label: 'Automatic' },
					{ value: 'manual', label: 'Manual' }
				]}
				onchange={(value) => {
					if (value === 'data') setVolumeScale('data');
					else {
						min = String($volumeLegend.min);
						max = String(
							$volumeLegend.max > $volumeLegend.min ? $volumeLegend.max : $volumeLegend.min + 1
						);
						updateRange();
					}
				}}
			/>{/if}
	</div>
    {#if $renderMode === 'glyph'}<div class="volume-controls__description">
        Arrow direction always uses the complete XYZ vector. Color by selects only the color; it does not change arrow orientation.
    </div>{/if}
	{#if field}<div class="volume-controls__description">
			{#if isVolume && $volumeProjection === 'average'}Arithmetic mean along {$volumeProjectionAxis.toUpperCase()},
				projected onto the existing surface. All preview layers contribute, including zero values;
				empty cells contribute zero.{:else}Local field values on {isVolume ? "the outer boundary and cavity walls" : "each displayed cell"}. Clipping exposes values inside the body.{/if}
		</div>{:else}<div class="volume-controls__description">
			Solid, opaque cells reveal geometry and cavities. Choose a field component to apply the 2D
			color map.
		</div>{/if}
	{#if scalar}
		{#if $volumeScaleMode === 'manual'}<div class="volume-controls__range">
				<label
					>Minimum<input
						aria-label="3D color minimum"
						type="number"
						step="any"
						bind:value={min}
						oninput={updateRange}
					/></label
				><label
					>Maximum<input
						aria-label="3D color maximum"
						type="number"
						step="any"
						bind:value={max}
						oninput={updateRange}
					/></label
				>{#if !valid}<span role="alert">Minimum must be smaller than maximum.</span>{/if}
			</div>{/if}
		<div class="volume-controls__legend">
			<span>{$volumeLegend.min.toPrecision(4)}</span>
			<div style:background={gradient} aria-label="3D color legend"></div>
			<span>{$volumeLegend.max.toPrecision(4)}</span><strong
				>{$previewState.quantity} · {colorMode.toUpperCase()}
				{$previewState.unit ? `(${$previewState.unit})` : ''}</strong
			>
		</div>
	{/if}
	{#if isVolume}<div class="volume-controls__footer">
		{#if field}<Toggle
				label="Surface lighting"
				checked={$volumeLighting}
				onchange={setVolumeLighting}
			/>{:else}<strong>Lit geometry</strong>{/if}<span
			>{field
				? $volumeLighting
					? 'Lighting changes perceived colors.'
					: 'Unlit colors match the 2D palette.'
				: 'Solid geometry uses lighting to reveal its shape.'}</span
		>
	</div>{/if}
	{#if !$previewState.allLayers && isVolume && $volumeProjection === 'average' && field}<div
			class="volume-controls__notice"
		>
			Waiting for all layers to compute the thickness average.
		</div>{/if}
	{#if ($previewState.transportSampling || 1) > 1}<div class="volume-controls__notice">
			Waiting for full preview data; sampled transfer can leave gaps.
		</div>{/if}
</details>

<style>
	.volume-controls {
		padding: 1rem;
		border-top: 1px solid var(--border-subtle);
		background: rgba(87, 200, 182, 0.025);
		display: block;
		gap: 0.85rem;
		flex-shrink: 0;
		min-width: 0;
	}
	.volume-controls[open] > :not(summary) {
		margin-top: 0.85rem;
	}
	.volume-controls__heading {
		cursor: pointer;
		list-style: none;
		display: flex;
		align-items: center;
		gap: 0.65rem;
	}
	.volume-controls__heading::-webkit-details-marker {
		display: none;
	}
	.volume-controls__heading:after {
		content: '⌄';
		color: var(--text-3);
	}
	.volume-controls:not([open]) .volume-controls__heading:after {
		content: '›';
	}
	.volume-controls__icon {
		font-size: 1.5rem;
		color: var(--accent);
	}
	.volume-controls__heading > div {
		display: grid;
		gap: 0.15rem;
		flex: 1;
	}
	.volume-controls__heading strong {
		font-size: 0.8rem;
		color: var(--text-1);
	}
	.volume-controls__heading div span {
		font-size: 0.65rem;
		color: var(--text-3);
	}
	.volume-controls__badge {
		font-size: 0.65rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-pill);
		padding: 0.3rem 0.6rem;
		color: var(--accent);
	}
	.volume-controls__grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.8rem;
	}
	.volume-controls__description,
	.volume-controls__footer > span {
		font-size: 0.7rem;
		color: var(--text-2);
		line-height: 1.5;
	}
	.volume-controls__footer {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.75rem;
	}
	.volume-controls__legend {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--text-2);
		flex-wrap: wrap;
	}
	.volume-controls__legend > div {
		flex: 1;
		min-width: 100px;
		height: 0.55rem;
		border-radius: var(--radius-pill);
	}
	.volume-controls__legend strong {
		flex-basis: 100%;
		font-size: 0.65rem;
		font-weight: 400;
		text-align: center;
	}
	.volume-controls__range {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.6rem;
	}
	.volume-controls__range label {
		display: grid;
		gap: 0.3rem;
		font-size: 0.7rem;
		color: var(--text-2);
		min-width: 0;
	}
	.volume-controls__range input {
		width: 100%;
		min-width: 0;
		background: var(--surface-1);
		color: var(--text-1);
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-sm);
		padding: 0.45rem 0.6rem;
	}
	.volume-controls__range > span {
		grid-column: 1/-1;
		color: var(--warning);
		font-size: 0.7rem;
	}
	.volume-controls__notice {
		font-size: 0.7rem;
		color: var(--warning);
	}
	@media (max-width: 650px) {
		.volume-controls__grid {
			grid-template-columns: minmax(0, 1fr);
		}
		.volume-controls {
			padding: 0.8rem;
			gap: 0.65rem;
		}
	}
</style>
