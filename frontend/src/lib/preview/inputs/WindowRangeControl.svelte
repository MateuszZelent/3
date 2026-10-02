<script lang="ts">
	import { moveWindow, type WindowRange } from '../preview2DWindow';
	type Props = {
		axis: string;
		extent: number;
		count: number;
		range: WindowRange;
		onchange: (range: WindowRange) => void;
	};
	let { axis, extent, count, range, onchange }: Props = $props();
	const step = $derived(1 / Math.max(count, 1));
	const distance = (fraction: number) => Number((fraction * extent).toPrecision(6));
	let track: HTMLDivElement;
	let drag: { x: number; range: WindowRange } | null = null;
	function move(event: PointerEvent) {
		if (!drag) return;
		onchange(moveWindow(drag.range, (event.clientX - drag.x) / track.clientWidth));
	}
</script>

<div class="window-range">
	<div class="window-range__label">
		<strong>{axis.toUpperCase()} window</strong><span
			>{distance(range[0])} – {distance(range[1])} nm</span
		>
	</div>
	<div class="window-range__track" bind:this={track}>
		<div
			class="window-range__selection"
			role="slider"
			tabindex="0"
			aria-label={`Move ${axis.toUpperCase()} window`}
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={Math.round(range[0] * 100)}
			aria-valuetext={`${distance(range[0])} to ${distance(range[1])} nanometres`}
			style={`left:${range[0] * 100}%;width:${(range[1] - range[0]) * 100}%`}
			onpointerdown={(e) => {
				if (e.button !== 0) return;
				drag = { x: e.clientX, range: [...range] };
				e.currentTarget.setPointerCapture(e.pointerId);
				e.preventDefault();
			}}
			onpointermove={move}
			onpointerup={() => {
				drag = null;
			}}
			onpointercancel={() => {
				drag = null;
			}}
			onlostpointercapture={() => {
				drag = null;
			}}
			onkeydown={(e) => {
				if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
					e.preventDefault();
					onchange(
						moveWindow(range, (e.key === 'ArrowLeft' ? -1 : 1) * step * (e.shiftKey ? 10 : 1))
					);
				}
			}}
		>
			<span aria-hidden="true">⋮⋮</span>
		</div>
		<input
			class="window-range__endpoint"
			aria-label={`${axis.toUpperCase()} window start`}
			type="range"
			min="0"
			max="1"
			{step}
			value={range[0]}
			disabled={count < 2}
			oninput={(e) =>
				onchange([Math.min(Number(e.currentTarget.value), range[1] - step), range[1]])}
		/>
		<input
			class="window-range__endpoint"
			aria-label={`${axis.toUpperCase()} window end`}
			type="range"
			min="0"
			max="1"
			{step}
			value={range[1]}
			disabled={count < 2}
			oninput={(e) =>
				onchange([range[0], Math.max(Number(e.currentTarget.value), range[0] + step)])}
		/>
	</div>
	<div class="window-range__extent">
		<span>0 nm</span><span>Drag the handles or slide the window</span><span>{distance(1)} nm</span>
	</div>
</div>

<style>
	.window-range {
		display: grid;
		gap: 0.6rem;
		min-width: 0;
	}
	.window-range__label,
	.window-range__extent {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.window-range__label strong {
		font-size: 0.72rem;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-2);
	}
	.window-range__label > span {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--accent);
	}
	.window-range__extent {
		font-size: 0.62rem;
		color: var(--text-3);
	}
	.window-range__track {
		position: relative;
		height: 1.7rem;
		margin: 0 0.45rem;
		border-radius: 0.45rem;
		background: var(--surface-3);
		border: 1px solid var(--border-subtle);
	}
	.window-range__selection {
		position: absolute;
		top: 0;
		height: 100%;
		display: grid;
		place-items: center;
		min-width: 1px;
		border-radius: 0.4rem;
		background: color-mix(in srgb, var(--accent) 20%, transparent);
		color: var(--accent);
		cursor: grab;
		touch-action: none;
	}
	.window-range__selection:active {
		cursor: grabbing;
	}
	.window-range__selection span {
		font-size: 0.7rem;
		letter-spacing: 2px;
		pointer-events: none;
	}
	.window-range__endpoint {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		margin: 0;
		background: transparent;
		appearance: none;
		pointer-events: none;
	}
	.window-range__endpoint::-webkit-slider-thumb {
		appearance: none;
		width: 0.8rem;
		height: 1.6rem;
		border-radius: 0.25rem;
		background: var(--accent);
		border: 2px solid var(--surface-1);
		pointer-events: auto;
		cursor: ew-resize;
	}
	.window-range__endpoint::-moz-range-thumb {
		width: 0.6rem;
		height: 1.4rem;
		border-radius: 0.25rem;
		background: var(--accent);
		border: 2px solid var(--surface-1);
		pointer-events: auto;
		cursor: ew-resize;
	}
	.window-range__endpoint::-moz-range-track {
		background: transparent;
	}
	.window-range__endpoint:disabled::-webkit-slider-thumb {
		cursor: default;
		opacity: 0.5;
	}
	.window-range__endpoint:focus-visible,
	.window-range__selection:focus-visible {
		outline: 2px solid var(--info);
		outline-offset: 3px;
	}
	@media (max-width: 450px) {
		.window-range__extent > span:nth-child(2) {
			display: none;
		}
	}
</style>
