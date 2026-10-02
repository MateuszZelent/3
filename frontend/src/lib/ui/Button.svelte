<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { StatusTone } from './types';

	type Variant = 'solid' | 'outline' | 'ghost' | 'subtle';
	type Size = 'sm' | 'md';

	type Props = {
		variant?: Variant;
		tone?: StatusTone;
		size?: Size;
		type?: 'button' | 'submit' | 'reset';
		disabled?: boolean;
		fullWidth?: boolean;
		className?: string;
		title?: string;
		ariaLabel?: string;
		onclick?: (event: MouseEvent) => void;
		children?: Snippet;
	};

	let {
		variant = 'subtle',
		tone = 'default',
		size = 'md',
		type = 'button',
		disabled = false,
		fullWidth = false,
		className = '',
		title = '',
		ariaLabel = '',
		onclick,
		children
	}: Props = $props();
</script>

<button
	type={type}
	class={`ui-button ui-button--${variant} ui-button--${size} ${fullWidth ? 'ui-button--full' : ''} ${className}`.trim()}
	data-tone={tone}
	{disabled}
	{title}
	aria-label={ariaLabel || undefined}
	onclick={onclick}
>
	{#if children}
		{@render children()}
	{/if}
</button>

<style>
	.ui-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		min-height: 2.5rem;
		padding: 0 0.95rem;
		border-radius: var(--radius-pill);
		border: 1px solid transparent;
		background: transparent;
		color: var(--text-1);
		font-weight: 600;
		letter-spacing: -0.01em;
		cursor: pointer;
		transition:
			transform var(--duration-fast) var(--easing-default),
			background var(--duration-fast) var(--easing-default),
			border-color var(--duration-fast) var(--easing-default),
			box-shadow var(--duration-fast) var(--easing-default),
			color var(--duration-fast) var(--easing-default);
	}

	.ui-button:hover:not(:disabled) {
		transform: translateY(-1px);
	}

	.ui-button:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}

	.ui-button:disabled {
		opacity: 0.45;
		cursor: not-allowed;
		transform: none;
	}

	.ui-button--full {
		width: 100%;
	}

	.ui-button--sm {
		min-height: 2.1rem;
		padding-inline: 0.8rem;
		font-size: 0.85rem;
	}

	.ui-button--solid {
		background: var(--surface-3);
		border-color: transparent;
		box-shadow: inset 0 1px 0 color-mix(in srgb, var(--text-1) 8%, transparent);
	}

	.ui-button--solid[data-tone='accent'],
	.ui-button--solid[data-tone='success'] {
		background: linear-gradient(135deg, var(--accent), var(--accent-strong));
		color: var(--on-accent);
	}

	.ui-button--solid[data-tone='info'] {
		background: linear-gradient(135deg, var(--info), #5f87ff);
		color: var(--on-accent);
	}

	.ui-button--solid[data-tone='warn'] {
		background: linear-gradient(135deg, var(--warn), #f39b3d);
		color: #201305;
	}

	.ui-button--solid[data-tone='danger'] {
		background: linear-gradient(135deg, var(--danger), #ff5d78);
		color: #22080b;
	}

	.ui-button--outline {
		border-color: var(--border);
		background: color-mix(in srgb, var(--text-1) 2%, transparent);
		color: var(--text-2);
	}

	.ui-button--outline:hover:not(:disabled),
	.ui-button--ghost:hover:not(:disabled),
	.ui-button--subtle:hover:not(:disabled) {
		border-color: var(--border-interactive);
		background: color-mix(in srgb, var(--info) 8%, transparent);
		color: var(--text-1);
	}

	.ui-button--outline[data-tone='accent'],
	.ui-button--ghost[data-tone='accent'] {
		color: var(--accent);
	}

	.ui-button--outline[data-tone='warn'],
	.ui-button--ghost[data-tone='warn'] {
		color: var(--warn);
	}

	.ui-button--outline[data-tone='danger'],
	.ui-button--ghost[data-tone='danger'] {
		color: var(--danger);
	}

	.ui-button--ghost {
		border-color: transparent;
		color: var(--text-2);
	}

	.ui-button--subtle {
		background: color-mix(in srgb, var(--text-1) 4.5%, transparent);
		border-color: color-mix(in srgb, var(--text-1) 4%, transparent);
		color: var(--text-2);
	}
</style>
