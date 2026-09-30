<script lang="ts">
	import SelectField from '$lib/ui/SelectField.svelte';
	import Toggle from '$lib/ui/Toggle.svelte';
	import { previewState as p } from '$api/incoming/preview';
	import {
		brightness,
		voxelOpaque,
		setVoxelOpaque,
		glyphSampling,
		setGlyphSampling,
		clipAxis,
		clipMin,
		clipMax,
		setClip,
		qualityLevel,
		renderMode,
		resetCamera,
		setBrightness,
		setQuality,
		setRenderMode,
		setVoxelColorMode,
		setVoxelGap,
		setVoxelOpacity,
		setVoxelSampling,
		setVoxelThreshold,
		setTopoEnabled,
		setTopoComponent,
		setTopoMultiplier,
		voxelColorMode,
		voxelGap,
		voxelOpacity,
		voxelSampling,
		voxelThreshold,
		topoEnabled,
		topoComponent,
		topoMultiplier,
		type Preview3DRenderMode,
		type QualityLevel,
		type VoxelColorMode,
		type VoxelSampling,
		type TopoComponent
	} from '$lib/preview/preview3D';

	let expanded = false;
	let brightnessVal: number;
	let opacityVal: number;
	let gapVal: number;
	let thresholdVal: number;

	$: brightnessVal = $brightness;
	$: opacityVal = $voxelOpacity;
	$: gapVal = $voxelGap;
	$: thresholdVal = $voxelThreshold;
	$: topoMulVal = $topoMultiplier;

	const qualityLevels: { key: QualityLevel; label: string }[] = [
		{ key: 'low', label: 'LOW' },
		{ key: 'high', label: 'HIGH' },
		{ key: 'ultra', label: 'ULTRA' }
	];

	const renderModes: { key: Preview3DRenderMode; label: string }[] = [
		{ key: 'glyph', label: 'ARROWS' },
		{ key: 'voxel', label: 'VOXEL' }
	];

	const colorModes: { key: VoxelColorMode; label: string }[] = [
		{ key: 'orientation', label: 'ORI' },
		{ key: 'x', label: 'X' },
		{ key: 'y', label: 'Y' },
		{ key: 'z', label: 'Z' }
	];

	const samplingModes: { key: VoxelSampling; label: string }[] = [
		{ key: 1, label: '1X' },
		{ key: 2, label: '2X' },
		{ key: 4, label: '4X' }
	];

	const topoComponents: { key: TopoComponent; label: string }[] = [
		{ key: 'x', label: 'X' },
		{ key: 'y', label: 'Y' },
		{ key: 'z', label: 'Z' }
	];

	$: isVisible = $p.nComp === 3 && $p.type === '3D';
</script>

{#if isVisible}
	<div class="toolbar" class:expanded>
		<button class="toggle-btn" onclick={() => (expanded = !expanded)} title="3D Controls">⚙</button>

		{#if expanded}
			<div class="toolbar-content">
				<div class="control-group">
					<div class="control-label">Render mode</div>
					<div class="btn-group btn-group--pair">
						{#each renderModes as { key, label }}
							<button
								class="seg-btn"
								class:active={$renderMode === key}
								onclick={() => setRenderMode(key)}
							>
								{label}
							</button>
						{/each}
					</div>
				</div>

				<div class="control-group">
					<div class="control-label">
						Brightness
						<span class="control-value">{brightnessVal.toFixed(1)}</span>
					</div>
					<input
						aria-label="3D preview brightness"
						type="range"
						min="0.3"
						max="3.0"
						step="0.1"
						value={brightnessVal}
						oninput={(event) => setBrightness(parseFloat(event.currentTarget.value))}
						class="slider"
					/>
				</div>

				<div class="control-group">
					<div class="control-label">Quality</div>
					<div class="btn-group btn-group--wide">
						{#each qualityLevels as { key, label }}
							<button
								class="seg-btn"
								class:active={$qualityLevel === key}
								onclick={() => setQuality(key)}
							>
								{label}
							</button>
						{/each}
					</div>
				</div>

				<div class="control-group">
					<div class="control-label">Section / ROI</div>
					<SelectField
						label="Section axis"
						value={$clipAxis}
						options={[
							{ value: 'none', label: 'Full field' },
							...['x', 'y', 'z'].map((value) => ({
								value,
								label: value.toUpperCase()
							}))
						]}
						onchange={(value) => setClip(value as 'none' | 'x' | 'y' | 'z', $clipMin, $clipMax)}
					/>
					{#if $clipAxis !== 'none'}
						<label class="control-label section-range"
							>From <input
								class="slider"
								aria-label="Section start"
								type="range"
								min="0"
								max="1"
								step="0.01"
								value={$clipMin}
								oninput={(e) => setClip($clipAxis, Number(e.currentTarget.value), $clipMax)}
							/></label
						>
						<label class="control-label section-range"
							>To <input
								class="slider"
								aria-label="Section end"
								type="range"
								min="0"
								max="1"
								step="0.01"
								value={$clipMax}
								oninput={(e) => setClip($clipAxis, $clipMin, Number(e.currentTarget.value))}
							/></label
						>
					{/if}
				</div>
				{#if $renderMode === 'glyph'}
					<div class="control-group">
						<div class="control-label">Arrow sampling</div>
						<div class="btn-group btn-group--wide">
							{#each samplingModes as { key, label }}<button
									class="seg-btn"
									class:active={$glyphSampling === key}
									onclick={() => setGlyphSampling(key)}>{label}</button
								>{/each}
						</div>
					</div>
				{/if}
				{#if $renderMode === 'voxel'}
					<Toggle label="Opaque voxels (fast)" checked={$voxelOpaque} onchange={setVoxelOpaque} />
					<div class="control-group">
						<div class="control-label">Color by</div>
						<div class="btn-group">
							{#each colorModes as { key, label }}
								<button
									class="seg-btn"
									class:active={$voxelColorMode === key}
									onclick={() => setVoxelColorMode(key)}
								>
									{label}
								</button>
							{/each}
						</div>
					</div>

					<div class="control-group">
						<div class="control-label">
							Opacity
							<span class="control-value">{opacityVal.toFixed(2)}</span>
						</div>
						<input
							aria-label="Voxel opacity"
							type="range"
							min="0.15"
							max="0.95"
							step="0.01"
							value={opacityVal}
							oninput={(event) => setVoxelOpacity(parseFloat(event.currentTarget.value))}
							class="slider"
						/>
					</div>

					<div class="control-group">
						<div class="control-label">
							Spacing
							<span class="control-value">{Math.round(gapVal * 100)}%</span>
						</div>
						<input
							aria-label="Voxel spacing"
							type="range"
							min="0.02"
							max="0.42"
							step="0.01"
							value={gapVal}
							oninput={(event) => setVoxelGap(parseFloat(event.currentTarget.value))}
							class="slider"
						/>
					</div>

					<div class="control-group">
						<div class="control-label">
							Min strength
							<span class="control-value">{thresholdVal.toFixed(2)}</span>
						</div>
						<input
							aria-label="Voxel threshold"
							type="range"
							min="0"
							max="0.95"
							step="0.01"
							value={thresholdVal}
							oninput={(event) => setVoxelThreshold(parseFloat(event.currentTarget.value))}
							class="slider"
						/>
					</div>

					<div class="control-group">
						<div class="control-label">Sampling</div>
						<div class="btn-group btn-group--wide">
							{#each samplingModes as { key, label }}
								<button
									class="seg-btn"
									class:active={$voxelSampling === key}
									onclick={() => setVoxelSampling(key)}
								>
									{label}
								</button>
							{/each}
						</div>
					</div>
				{/if}

				<div class="divider"></div>

				<!-- Topography -->
				<div class="control-group">
					<div class="control-label">Topography</div>
					<button
						class="action-btn"
						class:action-btn--topo-active={$topoEnabled}
						onclick={() => setTopoEnabled(!$topoEnabled)}
					>
						{$topoEnabled ? '⛰ ON' : 'OFF'}
					</button>
				</div>

				{#if $topoEnabled}
					<div class="control-group">
						<div class="control-label">Displace by</div>
						<div class="btn-group btn-group--wide">
							{#each topoComponents as { key, label }}
								<button
									class="seg-btn"
									class:active={$topoComponent === key}
									onclick={() => setTopoComponent(key)}
								>
									m{label}
								</button>
							{/each}
						</div>
					</div>

					<div class="control-group">
						<div class="control-label">
							Amplitude
							<span class="control-value">{topoMulVal.toFixed(1)}×</span>
						</div>
						<input
							aria-label="Topography amplitude"
							type="range"
							min="0.5"
							max="50"
							step="0.5"
							value={topoMulVal}
							oninput={(event) => setTopoMultiplier(parseFloat(event.currentTarget.value))}
							class="slider"
						/>
					</div>
				{/if}

				<button class="action-btn" onclick={resetCamera}>Reset Camera</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.toolbar {
		position: absolute;
		left: var(--space-sm);
		top: var(--space-sm);
		z-index: var(--z-sticky);
		display: flex;
		flex-direction: column;
	}

	.toggle-btn {
		width: 32px;
		height: 32px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-md);
		background: var(--surface-glass);
		border: 1px solid var(--border);
		color: var(--text-3);
		font-size: 16px;
		cursor: pointer;
		transition: all var(--duration-fast) var(--easing-default);
		backdrop-filter: blur(8px);
	}

	.toggle-btn:hover {
		background: var(--surface-3);
		color: var(--text-1);
	}

	.toolbar-content {
		margin-top: var(--space-xs);
		background: linear-gradient(180deg, rgba(12, 18, 31, 0.92), rgba(8, 12, 22, 0.92));
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: var(--space-sm);
		min-width: 200px;
		max-width: 220px;
		max-height: 360px;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		backdrop-filter: blur(14px);
		box-shadow: 0 20px 50px rgba(0, 0, 0, 0.28);
		scrollbar-width: thin;
		scrollbar-color: rgba(107, 167, 255, 0.3) transparent;
	}

	.control-group {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}

	.control-label {
		font-size: 11px;
		color: var(--text-2);
		display: flex;
		justify-content: space-between;
		align-items: center;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.control-value {
		color: var(--text-1);
		font-weight: 600;
		font-family: var(--font-mono);
	}

	.slider {
		width: 100%;
		height: 4px;
		-webkit-appearance: none;
		appearance: none;
		background: linear-gradient(90deg, rgba(87, 200, 182, 0.2), rgba(107, 167, 255, 0.24));
		border-radius: 999px;
		outline: none;
		cursor: pointer;
		border: none;
	}

	.slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--accent);
		border: 2px solid rgba(9, 14, 24, 0.9);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.28);
		cursor: pointer;
	}

	.btn-group {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		border-radius: var(--radius-md);
		overflow: hidden;
		border: 1px solid var(--border);
		background: rgba(255, 255, 255, 0.03);
	}

	.section-range {
		display: grid;
		grid-template-columns: 2.6rem minmax(0, 1fr);
		gap: 0.6rem;
	}

	.btn-group--pair {
		grid-template-columns: repeat(2, minmax(0, 1fr));
	}

	.btn-group--wide {
		grid-template-columns: repeat(3, minmax(0, 1fr));
	}

	.seg-btn {
		padding: 0.42rem 0;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.04em;
		color: var(--text-3);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: all var(--duration-fast) var(--easing-default);
	}

	.seg-btn:not(:last-child) {
		border-right: 1px solid var(--border);
	}

	.seg-btn:hover {
		background: rgba(107, 167, 255, 0.08);
		color: var(--text-2);
	}

	.seg-btn.active {
		background: linear-gradient(135deg, rgba(87, 200, 182, 0.92), rgba(56, 178, 162, 0.92));
		color: #08101d;
	}

	.action-btn--topo-active {
		background: linear-gradient(
			135deg,
			rgba(52, 211, 153, 0.85),
			rgba(16, 185, 129, 0.85)
		) !important;
		color: #08101d !important;
		border-color: rgba(52, 211, 153, 0.5) !important;
	}

	.divider {
		height: 1px;
		background: var(--border);
		margin: 2px 0;
	}

	.action-btn {
		padding: 0.48rem 0;
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.04em;
		color: var(--text-2);
		background: rgba(255, 255, 255, 0.035);
		border: 1px solid var(--border);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: all var(--duration-fast) var(--easing-default);
	}

	.action-btn:hover {
		background: rgba(107, 167, 255, 0.08);
		border-color: var(--border-interactive);
		color: var(--text-1);
	}

	@media (max-width: 767px) {
		.toolbar-content {
			min-width: 196px;
			max-width: 212px;
		}
	}
</style>
