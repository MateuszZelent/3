<script lang="ts">
	import { previewState } from '$api/incoming/preview';
	import SelectField from '$lib/ui/SelectField.svelte';
	import Toggle from '$lib/ui/Toggle.svelte';
	import TextField from '$lib/ui/TextField.svelte';
	import { postScale } from '$api/outgoing/preview';
	import {
		renderMode,
		brightness,
		setBrightness,
		volumeColorMode,
		volumeLighting,
		setVolumeLighting,
		glyphSampling,
		setGlyphSampling,
		voxelSampling,
		setVoxelSampling,
		voxelOpaque,
		setVoxelOpaque,
		voxelOpacity,
		setVoxelOpacity,
		voxelGap,
		setVoxelGap,
		voxelThreshold,
		setVoxelThreshold,
		topoEnabled,
		setTopoEnabled,
		topoComponent,
		setTopoComponent,
		topoMultiplier,
		setTopoMultiplier,
		type VoxelSampling,
		type TopoComponent
	} from '../preview3D';

	const isVolume = $derived($renderMode === 'volume');
	const isVoxel = $derived($renderMode === 'voxel');
	const lit = $derived(!isVolume || $volumeColorMode === 'geometry' || $volumeLighting);
	const sliders = $derived([
		{
			label: 'Lighting intensity',
			value: $brightness,
			min: 0.3,
			max: 3,
			step: 0.1,
			change: setBrightness,
			disabled: !lit
		},
		...(isVoxel
			? [
					{
						label: 'Voxel opacity',
						value: $voxelOpacity,
						min: 0.15,
						max: 0.95,
						step: 0.01,
						change: setVoxelOpacity,
						disabled: $voxelOpaque
					},
					{
						label: 'Voxel spacing',
						value: $voxelGap,
						min: 0.02,
						max: 0.42,
						step: 0.01,
						change: setVoxelGap,
						disabled: false
					},
					{
						label: 'Minimum vector strength',
						value: $voxelThreshold,
						min: 0,
						max: 0.95,
						step: 0.01,
						change: setVoxelThreshold,
						disabled: false
					}
				]
			: []),
		...(isVoxel && $topoEnabled
			? [
					{
						label: 'Topography amplitude',
						value: $topoMultiplier,
						min: 0.5,
						max: 50,
						step: 0.5,
						change: setTopoMultiplier,
						disabled: false
					}
				]
			: [])
	]);
</script>

<details class="render-controls">
	<summary
		>{isVolume
			? 'Surface appearance'
			: isVoxel
				? 'Voxel material & topography'
				: 'Arrow display'}</summary
	>
	<div class="render-controls__body">
		{#if isVolume}
			{#if $volumeColorMode !== 'geometry'}
				<Toggle label="Surface lighting" checked={$volumeLighting} onchange={setVolumeLighting} />
				<p>
					{lit ? 'Lighting changes perceived colors.' : 'Unlit colors preserve the field palette.'}
				</p>
			{:else}<p>Solid geometry uses lighting to reveal its shape.</p>{/if}
		{:else}
			<TextField
				label={$renderMode === 'glyph' ? 'Arrow length scale' : 'Vector normalization'}
				hint="0 = adaptive"
				type="number"
				min={0}
				step="any"
				value={$previewState.fixedScale ?? 0}
				onchange={(e) => postScale(Number((e.currentTarget as HTMLInputElement).value))}
			/>
			<SelectField
				label="Display sampling"
				value={$previewState.regionActive ? 1 : isVoxel ? $voxelSampling : $glyphSampling}
				disabled={$previewState.regionActive}
				options={[1, 2, 4].map((value) => ({ value: String(value), label: `${value}×` }))}
				onchange={(value) =>
					isVoxel
						? setVoxelSampling(Number(value) as VoxelSampling)
						: setGlyphSampling(Number(value) as VoxelSampling)}
			/>
			{#if $previewState.regionActive}<p>
					Local windows display every prepared cell. Set detail in the 3D render window.
				</p>{/if}
		{/if}
		{#if isVoxel}
			<Toggle label="Opaque voxels" checked={$voxelOpaque} onchange={setVoxelOpaque} />
			<Toggle label="Voxel topography" checked={$topoEnabled} onchange={setTopoEnabled} />
			{#if $topoEnabled}
				<SelectField
					label="Topography component"
					value={$topoComponent}
					options={['x', 'y', 'z'].map((value) => ({ value, label: value.toUpperCase() }))}
					onchange={(value) => setTopoComponent(value as TopoComponent)}
				/>
			{/if}
		{/if}
		{#each sliders as slider}
			<label class="render-slider">
				<span>{slider.label}<output>{slider.value.toFixed(2)}</output></span>
				<input
					type="range"
					aria-label={slider.label}
					value={slider.value}
					min={slider.min}
					max={slider.max}
					step={slider.step}
					disabled={slider.disabled}
					oninput={(e) => slider.change(Number(e.currentTarget.value))}
				/>
			</label>
		{/each}
	</div>
</details>

<style>
	.render-controls {
		border-top: 1px solid var(--border-subtle);
		color: var(--text-1);
	}
	summary {
		padding: 0.85rem 1rem;
		cursor: pointer;
		font-size: 0.8rem;
		font-weight: 600;
	}
	.render-controls__body {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		padding: 0 1rem 1rem;
	}
	.render-controls__body :global(.ui-toggle),
	p {
		grid-column: 1/-1;
	}
	p {
		margin: 0;
		font-size: 0.75rem;
		color: var(--text-2);
		line-height: 1.5;
	}
	.render-slider {
		display: grid;
		gap: 0.65rem;
		min-width: 0;
		font-size: 0.75rem;
		color: var(--text-2);
	}
	.render-slider > span {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
	}
	output {
		color: var(--text-1);
		font-family: var(--font-mono);
	}
	input {
		width: 100%;
		accent-color: var(--accent);
	}
	input:disabled {
		opacity: 0.45;
	}
	@media (max-width: 500px) {
		.render-controls__body {
			grid-template-columns: 1fr;
		}
	}
</style>
