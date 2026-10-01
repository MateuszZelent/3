<script lang="ts">
	import { meshState } from '$api/incoming/mesh';
	import { previewState } from '$api/incoming/preview';
	import Button from '$lib/ui/Button.svelte';
	import Toggle from '$lib/ui/Toggle.svelte';
	import TextField from '$lib/ui/TextField.svelte';
	import SegmentedControl from '$lib/ui/SegmentedControl.svelte';
	import SelectField from '$lib/ui/SelectField.svelte';
	import WindowRangeControl from './WindowRangeControl.svelte';
	import {
		regionDraft,
		regionPending,
		regionError,
		changeRegion,
		setRegionWindow,
		fullDomain
	} from '../preview3DRegion';
	import { fitPreviewRegion, setGlyphSampling, setVoxelSampling } from '../preview3D';
	let expanded = $state(false);
	const counts = $derived([$meshState.Nx, $meshState.Ny, $meshState.Nz]);
	const cells = $derived([$meshState.dx, $meshState.dy, $meshState.dz]);
	const sizes = $derived($regionDraft.end.map((n, a) => Math.max(n - $regionDraft.start[a], 1)));
	const nativePoints = $derived(sizes.reduce((n, v) => n * v, 1));
	function mode(value: string) {
		if (value === 'native') {
			setGlyphSampling(1);
			setVoxelSampling(1);
		}
		changeRegion({ enabled: true, mode: value as 'native' | 'custom' });
	}
</script>

<details class="region-controls" bind:open={expanded}>
	<summary
		><strong>Render window</strong><span
			>{$regionPending
				? 'Updating…'
				: $regionDraft.enabled
					? `${sizes.join(' × ')} source cells`
					: 'Full domain'} <span aria-hidden="true">⌄</span></span
		></summary
	>
	<div class="region-body">
		<div class="region-header">
			<Toggle
				label="Local 3D window"
				checked={$regionDraft.enabled}
				onchange={(enabled) => changeRegion({ enabled })}
			/>
			<span>Crop source cells before sampling · physical proportions</span>
			<Button size="sm" variant="ghost" onclick={fitPreviewRegion}>Fit camera</Button>
			<Button size="sm" variant="ghost" onclick={fullDomain}>Full domain</Button>
		</div>
		<div class="region-ranges">
			{#each ['x', 'y', 'z'] as axis, a}
				<div>
					<WindowRangeControl
						{axis}
						count={counts[a]}
						extent={counts[a] * cells[a] * 1e9}
						range={[
							$regionDraft.start[a] / Math.max(counts[a], 1),
							$regionDraft.end[a] / Math.max(counts[a], 1)
						]}
						onchange={(range) => setRegionWindow(a, range)}
					/>
					<div class="cell-bounds">
						<TextField
							label={`${axis.toUpperCase()} first cell`}
							type="number"
							min={0}
							max={$regionDraft.end[a] - 1}
							step={1}
							value={$regionDraft.start[a]}
							onchange={(e) => {
								const value = Number((e.target as HTMLInputElement).value);
								if (Number.isInteger(value) && value >= 0 && value < $regionDraft.end[a])
									setRegionWindow(a, [value / counts[a], $regionDraft.end[a] / counts[a]]);
							}}
						/>
						<TextField
							label={`${axis.toUpperCase()} end cell`}
							hint="exclusive"
							type="number"
							min={$regionDraft.start[a] + 1}
							max={counts[a]}
							step={1}
							value={$regionDraft.end[a]}
							onchange={(e) => {
								const value = Number((e.target as HTMLInputElement).value);
								if (Number.isInteger(value) && value > $regionDraft.start[a] && value <= counts[a])
									setRegionWindow(a, [$regionDraft.start[a] / counts[a], value / counts[a]]);
							}}
						/>
					</div>
				</div>
			{/each}
		</div>
		<div class="region-resolution">
			<SegmentedControl
				value={$regionDraft.mode}
				options={[
					{ value: 'native', label: 'Native cells' },
					{ value: 'custom', label: 'Custom resolution' }
				]}
				onchange={mode}
			/>
			<span
				>{nativePoints.toLocaleString()} source cells · {$previewState.regionActive
					? `${$previewState.appliedXChosenSize} × ${$previewState.appliedYChosenSize} × ${$previewState.appliedZChosenSize} loaded`
					: 'Enable to apply local resolution'}</span
			>
		</div>
		{#if $regionDraft.mode === 'custom'}<div class="region-custom">
				{#each ['X', 'Y', 'Z'] as axis, a}<TextField
						label={`Local ${axis} samples`}
						type="number"
						min={1}
						max={sizes[a]}
						step={1}
						value={Math.min($regionDraft.samples[a], sizes[a])}
						onchange={(e) => {
							const n = Number((e.target as HTMLInputElement).value);
							if (Number.isInteger(n) && n >= 1 && n <= sizes[a]) {
								const samples = [...$regionDraft.samples] as [number, number, number];
								samples[a] = n;
								changeRegion({ enabled: true, samples });
							}
						}}
					/>{/each}
				<SelectField
					label="Local point budget"
					value={$regionDraft.maxPoints}
					options={[65536, 262144, 1000000].map((value) => ({
						value: String(value),
						label: value.toLocaleString()
					}))}
					onchange={(value) => changeRegion({ enabled: true, maxPoints: Number(value) })}
				/>
			</div>{/if}
		<p>
			Drag the filled range to pan, or use arrow keys (Shift: 10 cells). Bounds are measured from
			the mesh edge. Surface and thickness averages use this window. Native cells are exact up to
			1,000,000 points; larger windows are averaged.
		</p>
		{#if $regionError}<p role="alert">
				{$regionError} <button onclick={() => changeRegion({})}>Retry</button>
			</p>{/if}
	</div>
</details>

<style>
	.region-controls {
		border-top: 1px solid var(--border-subtle);
		background: var(--surface-1);
		color: var(--text-1);
	}
	summary {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem 1rem;
		cursor: pointer;
		font-size: 0.75rem;
		list-style: none;
	}
	summary > span {
		color: var(--text-2);
		font-size: 0.7rem;
	}
	.region-body {
		display: grid;
		gap: 1rem;
		padding: 0.25rem 1rem 1rem;
		max-height: 320px;
		overflow: auto;
	}
	.region-header,
	.region-resolution {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.75rem;
	}
	.region-header > span,
	.region-resolution > span,
	p {
		font-size: 0.7rem;
		color: var(--text-2);
	}
	.region-header > span {
		flex: 1;
	}
	.region-ranges {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 1.25rem;
	}
	.cell-bounds {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.5rem;
		margin-top: 0.6rem;
	}
	.region-custom {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0.75rem;
	}
	p {
		margin: 0;
		line-height: 1.5;
	}
	@media (max-width: 650px) {
		.region-ranges,
		.region-custom {
			grid-template-columns: 1fr;
		}
		.region-body {
			max-height: 230px;
		}
		.region-header > span {
			flex-basis: 100%;
		}
	}
</style>
