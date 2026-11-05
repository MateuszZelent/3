<script lang="ts">
	import { consoleState } from '$api/incoming/console';
	import { consoleOperationBusy, postCommand } from '$api/outgoing/console';
	import { headerState } from '$api/incoming/header';
	import { postBreak, solverOperationBusy } from '$api/outgoing/solver';
	import { connectionState } from '$api/websocket';
	import Button from '$lib/ui/Button.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import Panel from '$lib/ui/Panel.svelte';
	import { panelPreferences, toggleCollapsed, visiblePanels } from '$lib/ui/preferences';
	import { visibleConsoleFragment } from '$lib/console-history';
	import { tokenizeConsoleText, type ConsoleTokenNode } from '$lib/console-tokens';
	import { tick } from 'svelte';

	let command = $state('');
	let commandPending = $state(false);
	let commandHistory = $state<string[]>([]);
	let historyIndex = $state(-1);
	let codeDiv = $state<HTMLDivElement | null>(null);

	const displayedConsole = $derived(
		$visiblePanels.console ? visibleConsoleFragment($consoleState.hist) : null
	);
	const highlightedConsole = $derived(
		displayedConsole?.text ? tokenizeConsoleText(displayedConsole.text) : []
	);

	async function submitCommand() {
		const next = command.trim();
		if (!next || commandPending || $consoleOperationBusy || $connectionState === 'disconnected') {
			return;
		}
		commandPending = true;
		try {
			const result = await postCommand(next);
			if (!result.ok) return;
			commandHistory = [next, ...commandHistory.filter((entry) => entry !== next)].slice(0, 30);
			historyIndex = -1;
			if (command.trim() === next) command = '';
		} finally {
			commandPending = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			event.preventDefault();
			submitCommand();
			return;
		}

		if (event.key === 'ArrowUp') {
			event.preventDefault();
			if (!commandHistory.length) {
				return;
			}
			historyIndex = Math.min(historyIndex + 1, commandHistory.length - 1);
			command = commandHistory[historyIndex] ?? command;
		}

		if (event.key === 'ArrowDown') {
			event.preventDefault();
			if (!commandHistory.length) {
				return;
			}
			historyIndex = Math.max(historyIndex - 1, -1);
			command = historyIndex === -1 ? '' : (commandHistory[historyIndex] ?? '');
		}
	}

	$effect(() => {
		void highlightedConsole;
		if (!$visiblePanels.console) return;
		const output = codeDiv;
		if (!output) return;
		let cancelled = false;
		let frame: number | undefined;
		// Wait for highlighted HTML to reach the DOM, then use its final layout.
		tick().then(() => {
			if (cancelled) return;
			frame = requestAnimationFrame(() => {
				output.scrollTop = output.scrollHeight;
			});
		});
		return () => {
			cancelled = true;
			if (frame !== undefined) cancelAnimationFrame(frame);
		};
	});
</script>

{#snippet renderConsoleTokens(tokens: ConsoleTokenNode[])}
	{#each tokens as token, index (index)}
		{#if token.kind === 'text'}
			{token.text}
		{:else}
			<span class={token.className}>{@render renderConsoleTokens(token.children)}</span>
		{/if}
	{/each}
{/snippet}

<Panel
	title="Console"
	subtitle="Terminal-like command channel with history and sticky input."
	panelId="console"
	eyebrow="Diagnostics"
	collapsible={true}
	collapsed={$panelPreferences.collapsedByPanel.console}
	onToggleCollapse={() => toggleCollapsed('console')}
>
	<div class="console-shell">
		<div class="console-shell__output" bind:this={codeDiv}>
			{#if !$consoleState.hist}
				<EmptyState
					title={$connectionState === 'disconnected' ? 'Console offline' : 'Console is ready'}
					description={$connectionState === 'disconnected'
						? 'Reconnect to the backend to restore the command channel.'
						: 'Run commands to inspect or steer the simulation engine.'}
					tone={$connectionState === 'disconnected' ? 'warn' : 'info'}
					compact={true}
				/>
			{:else}
				{#if displayedConsole?.omitted}
					<p class="console-shell__truncation" role="note">
						Showing recent console output. Older output omitted from this view.
					</p>
				{/if}
				<div class="console-shell__code">{@render renderConsoleTokens(highlightedConsole)}</div>
			{/if}
		</div>

		<div class="console-shell__input">
			<input
				placeholder={$connectionState === 'connected'
					? 'Run a command. Use ↑ and ↓ for history.'
					: 'Console disabled while disconnected.'}
				value={command}
				oninput={(event) => (command = (event.currentTarget as HTMLInputElement).value)}
				onkeydown={handleKeydown}
				disabled={$connectionState === 'disconnected'}
			/>
			<Button
				variant="solid"
				tone="accent"
				onclick={submitCommand}
				disabled={$connectionState === 'disconnected' || commandPending || $consoleOperationBusy}
			>
				{commandPending ? 'Sending…' : 'Send'}
			</Button>
			{#if $consoleOperationBusy || $solverOperationBusy || $headerState.status === 'running'}
				<Button variant="outline" tone="danger" onclick={postBreak}>Break</Button>
			{/if}
		</div>
	</div>
</Panel>

<style>
	.console-shell {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
		min-height: 0;
	}

	.console-shell__output {
		min-height: var(--terminal-height);
		max-height: var(--terminal-height);
		overflow: auto;
		padding: 1rem;
		border-radius: var(--radius-md);
		border: 1px solid var(--border-subtle);
		background: linear-gradient(180deg, rgba(8, 12, 22, 0.96), rgba(7, 10, 18, 0.98)), #060911;
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
	}

	.console-shell__code {
		white-space: pre-wrap;
		font-family: 'IBM Plex Mono', monospace;
		font-size: 0.85rem;
		line-height: 1.6;
		color: var(--text-1);
	}

	.console-shell__truncation {
		margin: 0 0 0.7rem;
		color: var(--text-3);
		font-size: 0.75rem;
	}

	.console-shell__input {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto auto;
		gap: 0.75rem;
		position: sticky;
		bottom: 0;
	}

	.console-shell__input input {
		min-height: 2.8rem;
		padding: 0 0.95rem;
		border-radius: var(--radius-md);
		border: 1px solid var(--border-subtle);
		background: rgba(255, 255, 255, 0.03);
		color: var(--text-1);
		outline: none;
	}

	.console-shell__input input:focus {
		border-color: var(--border-interactive);
		box-shadow: var(--focus-ring);
	}

	.console-shell__input input::placeholder {
		color: var(--text-3);
	}

	:global(.token.comment),
	:global(.token.prolog),
	:global(.token.doctype),
	:global(.token.cdata) {
		color: #7b8fb6;
	}

	:global(.token.keyword) {
		color: #7cb5ff;
	}

	:global(.token.string),
	:global(.token.attr-value) {
		color: #7ad5a4;
	}

	:global(.token.number),
	:global(.token.boolean) {
		color: #ffbf73;
	}
</style>
