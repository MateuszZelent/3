<script lang="ts">
	import { previewState } from '$api/incoming/preview';
	import { meshState } from '$api/incoming/mesh';
	import Toggle from '$lib/ui/Toggle.svelte';
	import Button from '$lib/ui/Button.svelte';
	import WindowRangeControl from './WindowRangeControl.svelte';
	import { previewPlaneAxes } from '../preview2DCoordinates';
	import { preview2DWindow } from '../preview2DWindow';
	import {
		preview2DAutoscale,
		setPreview2DAutoscale,
		setPreview2DWindow,
		resetPreview2DWindow
	} from '../preview2D';
	const axes = $derived(previewPlaneAxes($previewState.plane));
	const extents = $derived({
		x: $meshState.Nx * $meshState.dx * 1e9,
		y: $meshState.Ny * $meshState.dy * 1e9,
		z: $meshState.Nz * $meshState.dz * 1e9
	});
</script>

<div class="plane-window-controls">
	<div class="plane-window-controls__header">
		<Toggle label="Autoscale" checked={$preview2DAutoscale} onchange={setPreview2DAutoscale} /><span
			>{$preview2DAutoscale
				? 'Fill the window · stretched proportions'
				: 'Physical proportions'}</span
		><Button size="sm" variant="ghost" onclick={resetPreview2DWindow}>Full window</Button>
	</div>
	<div class="plane-window-controls__ranges">
		<WindowRangeControl
			axis={axes.u}
			extent={extents[axes.u]}
			count={$previewState.appliedPlaneUSize || $previewState.appliedXChosenSize || 1}
			range={$preview2DWindow.u}
			onchange={(range) => setPreview2DWindow('u', range)}
		/>
		<WindowRangeControl
			axis={axes.v}
			extent={extents[axes.v]}
			count={$previewState.appliedPlaneVSize || $previewState.appliedYChosenSize || 1}
			range={$preview2DWindow.v}
			onchange={(range) => setPreview2DWindow('v', range)}
		/>
	</div>
</div>

<style>
	.plane-window-controls {
		padding: 0.9rem 1rem;
		display: grid;
		gap: 0.9rem;
		border-bottom: 1px solid var(--border-subtle);
		background: rgba(255, 255, 255, 0.015);
	}
	.plane-window-controls__header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}
	.plane-window-controls__header > span {
		font-size: 0.7rem;
		color: var(--text-2);
		flex: 1;
	}
	.plane-window-controls__ranges {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 1.4rem;
	}
	@media (max-width: 650px) {
		.plane-window-controls__ranges {
			grid-template-columns: 1fr;
			gap: 1rem;
		}
		.plane-window-controls__header > span {
			order: 3;
			flex-basis: 100%;
		}
	}
</style>
