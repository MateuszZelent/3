<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Button from '$lib/ui/Button.svelte';
	type Progress = {
		id: number;
		active: boolean;
		failed: boolean;
		stage: string;
		percent: number;
		startedAt: number;
		logs: string[];
	};
	let progress = $state<Progress | null>(null);
	let minimized = $state(false);
	let visible = $state(false);
	let now = $state(Date.now());
	let unavailable = $state(false);
	let logArea = $state<HTMLPreElement>();
	onMount(() => {
		let stopped = false;
		let timer: ReturnType<typeof setTimeout>;
		let closeTimer: ReturnType<typeof setTimeout>;
		let controller: AbortController | undefined;
		async function poll() {
			if (stopped || document.hidden || controller) return;
			const request = new AbortController();
			controller = request;
			// Keep initialization responsive without continuously polling idle engines.
			let delay = 5000;
			const deadline = setTimeout(() => request.abort(), 3000);
			try {
				const response = await fetch('./api/demag/progress', {
					signal: request.signal,
					cache: 'no-store'
				});
				if (!response.ok) throw new Error('Progress unavailable');
				const next: Progress = await response.json();
				if (stopped || controller !== request || document.hidden) return;
				delay = next.active ? 400 : 5000;
				unavailable = false;
				if (next.id && next.id !== progress?.id && (next.active || next.failed)) {
					clearTimeout(closeTimer);
					visible = true;
					minimized = false;
				}
				if (progress?.active && !next.active && !next.failed) {
					closeTimer = setTimeout(() => {
						visible = false;
					}, 2500);
				}
				progress = next;
				await tick();
				if (logArea) logArea.scrollTop = logArea.scrollHeight;
			} catch {
				if (!stopped && controller === request && !document.hidden) unavailable = true;
			} finally {
				clearTimeout(deadline);
				if (controller === request) {
					controller = undefined;
					if (!stopped && !document.hidden) timer = setTimeout(poll, delay);
				}
			}
		}
		function visibilityChanged() {
			clearTimeout(timer);
			if (document.hidden) {
				const request = controller;
				controller = undefined;
				request?.abort();
			} else {
				void poll();
			}
		}
		document.addEventListener('visibilitychange', visibilityChanged);
		void poll();
		const clock = setInterval(() => {
			now = Date.now();
		}, 500);
		return () => {
			stopped = true;
			document.removeEventListener('visibilitychange', visibilityChanged);
			controller?.abort();
			controller = undefined;
			clearTimeout(timer);
			clearTimeout(closeTimer);
			clearInterval(clock);
		};
	});
</script>

{#if visible && progress}
	<aside
		class:compact={minimized}
		class="demag-progress"
		aria-label="Demagnetization initialization"
	>
		<div class="heading">
			<strong
				>{progress.active
					? 'Preparing demagnetization'
					: progress.failed
						? 'Initialization failed'
						: 'Demagnetization ready'}</strong
			>
			<Button
				size="sm"
				variant="ghost"
				onclick={() => {
					if (progress?.active) minimized = !minimized;
					else visible = false;
				}}
			>
				{progress.active ? (minimized ? 'Show logs' : 'Minimize') : 'Close'}
			</Button>
		</div>
		{#if !minimized}
			<p role="status">
				{progress.stage.trim()} · {Math.max(0, (now - progress.startedAt) / 1000).toFixed(0)} s
			</p>
			{#if progress.active}<progress
					max="100"
					value={progress.percent >= 0 ? progress.percent : undefined}
					aria-label="Current stage progress"
				></progress>{/if}
			{#if unavailable}<p role="status">Waiting for a progress update…</p>{/if}
			<pre bind:this={logArea} aria-label="Kernel initialization logs">{progress.logs.join(
					'\n'
				)}</pre>
		{/if}
	</aside>
{/if}

<style>
	.demag-progress {
		position: fixed;
		right: 1rem;
		bottom: 1rem;
		z-index: var(--z-modal);
		width: min(36rem, calc(100vw - 2rem));
		padding: 1rem;
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-1);
		box-shadow: var(--shadow-panel);
	}
	.heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		font-size: 0.85rem;
	}
	p {
		font-size: 0.75rem;
		overflow-wrap: anywhere;
		color: var(--text-2);
	}
	progress {
		width: 100%;
		height: 0.5rem;
		accent-color: var(--accent);
	}
	pre {
		max-height: min(16rem, 40vh);
		overflow: auto;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-family: var(--font-mono);
		font-size: 0.68rem;
		line-height: 1.6;
		margin-bottom: 0;
	}
	.compact {
		width: auto;
	}
</style>
