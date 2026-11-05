<script lang="ts">
	import { setAlert } from '$lib/alerts/alert';
	type Props = {
		label: string;
		checked?: boolean;
		onchange?: (next: boolean) => unknown;
		disabled?: boolean;
	};

	let { label, checked = false, onchange, disabled = false }: Props = $props();
	let pending = $state(false);

	async function handleChange(event: Event) {
		const control = event.currentTarget as HTMLInputElement;
		if (pending || disabled) {
			control.checked = checked;
			return;
		}
		pending = true;
		try {
			const result = await onchange?.(control.checked);
			if (
				result === false ||
				(result && typeof result === 'object' && 'ok' in result && result.ok === false)
			)
				control.checked = checked;
		} catch (error) {
			control.checked = checked;
			setAlert(error instanceof Error ? error.message : 'Unable to apply setting');
		} finally {
			pending = false;
		}
	}
</script>

<label class="ui-toggle">
	<input type="checkbox" {checked} disabled={disabled || pending} onchange={handleChange} />
	<span class="ui-toggle__track" aria-hidden="true">
		<span class="ui-toggle__thumb"></span>
	</span>
	<span class="ui-toggle__label">{label}</span>
</label>

<style>
	.ui-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.75rem;
		cursor: pointer;
		color: var(--text-2);
	}
	.ui-toggle:has(input:disabled) {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.ui-toggle input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.ui-toggle__track {
		position: relative;
		width: 2.7rem;
		height: 1.55rem;
		border-radius: var(--radius-pill);
		background: color-mix(in srgb, var(--text-1) 8%, transparent);
		border: 1px solid var(--border-subtle);
		transition: background var(--duration-fast) var(--easing-default);
	}

	.ui-toggle__thumb {
		position: absolute;
		top: 0.16rem;
		left: 0.16rem;
		width: 1.05rem;
		height: 1.05rem;
		border-radius: 50%;
		background: var(--text-2);
		transition:
			transform var(--duration-fast) var(--easing-spring),
			background var(--duration-fast) var(--easing-default);
	}

	input:checked + .ui-toggle__track {
		background: color-mix(in srgb, var(--accent) 24%, transparent);
	}

	input:checked + .ui-toggle__track .ui-toggle__thumb {
		transform: translateX(1.1rem);
		background: var(--accent);
	}

	.ui-toggle__label {
		font-size: 0.92rem;
	}
</style>
