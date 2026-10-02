<script lang="ts">
	type Props = {
		label?: string;
		value?: string | number;
		type?: string;
		placeholder?: string;
		min?: number;
		max?: number;
		step?: number | 'any';
		hint?: string;
		unit?: string;
		readonly?: boolean;
		disabled?: boolean;
		mono?: boolean;
		inputMode?: 'none' | 'text' | 'tel' | 'url' | 'email' | 'numeric' | 'decimal' | 'search';
		name?: string;
		oninput?: (event: Event) => void;
		onchange?: (event: Event) => void;
		onkeydown?: (event: KeyboardEvent) => void;
	};

	let {
		label = '',
		value = '',
		type = 'text',
		placeholder = '',
		min,
		max,
		step,
		hint = '',
		unit = '',
		readonly = false,
		disabled = false,
		mono = false,
		inputMode = undefined,
		name = '',
		oninput,
		onchange,
		onkeydown
	}: Props = $props();
</script>

<label class="ui-textfield">
	{#if label}
		<span class="ui-textfield__label">
			<span>{label}</span>
			{#if hint}
				<span class="ui-textfield__hint">{hint}</span>
			{/if}
		</span>
	{/if}
	<span class="ui-textfield__control" data-readonly={readonly}>
		<input
			aria-label={label || undefined}
			{name}
			{type}
			{placeholder}
			{min}
			{max}
			{step}
			{readonly}
			{disabled}
			inputmode={inputMode}
			{value}
			data-mono={mono}
			{oninput}
			{onchange}
			{onkeydown}
		/>
		{#if unit}
			<span class="ui-textfield__unit">{unit}</span>
		{/if}
	</span>
</label>

<style>
	.ui-textfield {
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
		min-width: 0;
	}

	.ui-textfield__label {
		display: flex;
		justify-content: space-between;
		gap: 0.75rem;
		font-size: 0.76rem;
		font-weight: 600;
		color: var(--text-3);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.ui-textfield__hint {
		text-transform: none;
		letter-spacing: normal;
		font-size: 0.76rem;
	}

	.ui-textfield__control {
		display: flex;
		align-items: center;
		min-height: 2.85rem;
		gap: 0.7rem;
		padding: 0 0.9rem;
		border-radius: var(--radius-md);
		border: 1px solid var(--border-subtle);
		background: color-mix(in srgb, var(--text-1) 3%, transparent);
		transition:
			border-color var(--duration-fast) var(--easing-default),
			box-shadow var(--duration-fast) var(--easing-default),
			background var(--duration-fast) var(--easing-default);
	}

	.ui-textfield__control:focus-within {
		border-color: var(--border-interactive);
		box-shadow: var(--focus-ring);
		background: color-mix(in srgb, var(--text-1) 4%, transparent);
	}

	.ui-textfield__control[data-readonly='true'] {
		background: color-mix(in srgb, var(--text-1) 2%, transparent);
	}

	.ui-textfield input {
		width: 100%;
		min-width: 0;
		background: transparent;
		border: 0;
		outline: none;
		color: var(--text-1);
	}

	.ui-textfield input::placeholder {
		color: var(--text-3);
	}

	.ui-textfield input[data-mono='true'] {
		font-family: 'IBM Plex Mono', monospace;
	}

	.ui-textfield__unit {
		color: var(--text-3);
		font-size: 0.82rem;
	}
</style>
