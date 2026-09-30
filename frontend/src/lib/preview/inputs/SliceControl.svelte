<script lang="ts">
	import { previewSampleCoordinateNm } from '../preview2DCoordinates';
	type Props = {
		axis: string;
		count: number;
		cellSize: number;
		value: number;
		onchange: (index: number) => void;
	};
	let { axis, count, cellSize, value, onchange }: Props = $props();
	let draft = $state(0);
	let editing = $state(false);
	const last = $derived(Math.max(count - 1, 0));
	$effect(() => {
		if (!editing) draft = Math.min(Math.max(value, 0), last);
	});
	const position = $derived(
		previewSampleCoordinateNm(draft, Math.max(count, 1), cellSize * Math.max(count, 1) * 1e9)
	);
	function commit(index: number) {
		if (!Number.isInteger(index) || index < 0 || index > last) {
			editing = false;
			draft = value;
			return;
		}
		draft = index;
		editing = false;
		onchange(index);
	}
</script>

<div class="slice-control">
	<div class="slice-control__heading">
		<span class="slice-control__label">Cut along {axis.toUpperCase()}</span>
		<span class="slice-control__position"
			>{axis} = {Number.isFinite(position) ? Number(position.toPrecision(6)) : '—'}
			<span>nm</span></span
		>
	</div>
	<div class="slice-control__inputs">
		<button
			type="button"
			aria-label="Previous layer"
			disabled={draft === 0 || count < 2}
			onclick={() => commit(draft - 1)}>−</button
		>
		<input
			class="slice-control__range"
			aria-label={`${axis.toUpperCase()} slice`}
			type="range"
			min="0"
			max={last}
			step="1"
			value={draft}
			disabled={count < 2}
			oninput={(event) => {
				editing = true;
				draft = Number(event.currentTarget.value);
			}}
			onchange={(event) => commit(Number(event.currentTarget.value))}
			onblur={() => {
				editing = false;
			}}
		/>
		<button
			type="button"
			aria-label="Next layer"
			disabled={draft === last || count < 2}
			onclick={() => commit(draft + 1)}>+</button
		>
		<label class="slice-control__index"
			><span>Index</span><input
				aria-label="Slice index"
				type="number"
				min="0"
				max={last}
				step="1"
				value={draft}
				onfocus={() => {
					editing = true;
				}}
				onchange={(event) => commit(Number(event.currentTarget.value))}
				onblur={() => {
					editing = false;
					draft = value;
				}}
			/></label
		>
	</div>
	<div class="slice-control__extent">
		<span>0</span><span>Cell center · {count} {count === 1 ? 'layer' : 'layers'}</span><span
			>{last}</span
		>
	</div>
</div>

<style>
	.slice-control {
		display: grid;
		gap: 0.65rem;
		padding: 1rem 1.1rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: linear-gradient(100deg, rgba(87, 200, 182, 0.065), rgba(107, 167, 255, 0.035));
		min-width: 0;
	}
	.slice-control__heading,
	.slice-control__extent {
		display: flex;
		justify-content: space-between;
		gap: 0.75rem;
		align-items: center;
	}
	.slice-control__label {
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--text-2);
	}
	.slice-control__position {
		font-family: var(--font-mono);
		color: var(--accent);
		font-size: 0.85rem;
	}
	.slice-control__position span,
	.slice-control__extent {
		color: var(--text-3);
		font-size: 0.72rem;
	}
	.slice-control__inputs {
		display: grid;
		grid-template-columns: 1.85rem minmax(0, 1fr) 1.85rem auto;
		align-items: center;
		gap: 0.75rem;
	}
	button {
		height: 1.85rem;
		border: 1px solid var(--border-subtle);
		border-radius: 0.5rem;
		color: var(--text-2);
		background: var(--surface-2);
		cursor: pointer;
	}
	button:hover:not(:disabled) {
		border-color: var(--accent);
		color: var(--accent);
	}
	button:disabled {
		opacity: 0.35;
		cursor: default;
	}
	.slice-control__range {
		width: 100%;
		min-width: 0;
		accent-color: var(--accent);
		cursor: pointer;
	}
	.slice-control__index {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.35rem 0.5rem;
		border: 1px solid var(--border-subtle);
		border-radius: 0.5rem;
		font-size: 0.72rem;
		color: var(--text-3);
	}
	.slice-control__index input {
		width: 3.7rem;
		background: transparent;
		color: var(--text-1);
		border: 0;
		font-family: var(--font-mono);
	}
	input:focus-visible,
	button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
</style>
