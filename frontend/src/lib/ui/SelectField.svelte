<script lang="ts">
	import { setAlert } from '$lib/alerts/alert';
	export type SelectOption = {
		value: string;
		label: string;
		group?: string;
		disabled?: boolean;
	};

	type Props = {
		label?: string;
		value: string | number;
		options: SelectOption[];
		hint?: string;
		disabled?: boolean;
		onchange?: (value: string) => unknown;
	};

	let { label = '', value, options, hint = '', disabled = false, onchange }: Props = $props();
	let pending = $state(false);
	let select = $state<HTMLSelectElement>();
	$effect(() => {
		void options;
		if (select) select.value = String(value);
	});

	async function handleChange(event: Event) {
		const control = event.currentTarget as HTMLSelectElement;
		if (pending || disabled) {
			control.value = String(value);
			return;
		}
		pending = true;
		try {
			const result = await onchange?.(control.value);
			if (result && typeof result === 'object' && 'ok' in result && result.ok === false)
				control.value = String(value);
		} catch (error) {
			control.value = String(value);
			setAlert(error instanceof Error ? error.message : 'Unable to apply selection');
		} finally {
			pending = false;
		}
	}
</script>

<label class="ui-select">
	{#if label}
		<span class="ui-select__label">
			<span>{label}</span>
			{#if hint}
				<span class="ui-select__hint">{hint}</span>
			{/if}
		</span>
	{/if}
	<span class="ui-select__control">
		<select
			bind:this={select}
			disabled={disabled || pending}
			aria-label={label || undefined}
			value={String(value)}
			onchange={handleChange}
		>
			{#each options as option, index (index)}
				<option value={option.value} disabled={option.disabled}>
					{option.group ? `${option.group} / ${option.label}` : option.label}
				</option>
			{/each}
		</select>
		<span class="ui-select__chevron" aria-hidden="true">⌄</span>
	</span>
</label>

<style>
	.ui-select {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
		min-width: 0;
	}

	.ui-select__label {
		display: flex;
		justify-content: space-between;
		gap: 0.75rem;
		font-size: 0.76rem;
		font-weight: 600;
		color: var(--text-3);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.ui-select__hint {
		text-transform: none;
		letter-spacing: normal;
		font-size: 0.76rem;
	}

	.ui-select__control {
		position: relative;
		display: flex;
		align-items: center;
		min-height: 2.85rem;
		padding: 0 0.9rem;
		border-radius: var(--radius-md);
		border: 1px solid var(--border-subtle);
		background: color-mix(in srgb, var(--text-1) 3%, transparent);
	}

	.ui-select__control:focus-within {
		border-color: var(--border-interactive);
		box-shadow: var(--focus-ring);
	}

	.ui-select select {
		width: 100%;
		min-width: 0;
		border: 0;
		background: transparent;
		color: var(--text-1);
		color-scheme: dark;
		outline: none;
		padding-right: 1.2rem;
		appearance: none;
	}

	.ui-select select option {
		background: var(--surface-2, #141f33);
		color: var(--text-1, #edf3fb);
	}

	.ui-select__chevron {
		position: absolute;
		right: 0.95rem;
		color: var(--text-3);
		pointer-events: none;
	}
</style>
