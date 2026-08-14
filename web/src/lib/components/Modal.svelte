<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open = false,
		title,
		eyebrow = 'Edit record',
		children,
		onclose
	}: { open?: boolean; title: string; eyebrow?: string; children: Snippet; onclose: () => void } = $props();
</script>

{#if open}
	<div class="modal-backdrop" role="presentation" onclick={(event) => event.target === event.currentTarget && onclose()}>
		<div class="modal" role="dialog" aria-modal="true" aria-label={title} tabindex="-1">
			<div class="modal-header">
				<div><span class="eyebrow">{eyebrow}</span><h2>{title}</h2></div>
				<button class="icon-button" aria-label="Close dialog" onclick={onclose}>×</button>
			</div>
			<div class="modal-content">{@render children()}</div>
		</div>
	</div>
{/if}

<style>
	.modal-backdrop { position: fixed; inset: 0; z-index: 50; display: grid; place-items: center; padding: 20px; background: rgba(2,6,10,.78); backdrop-filter: blur(8px); }
	.modal { width: min(620px, 100%); max-height: min(760px, 92vh); overflow: auto; background: #121c26; border: 1px solid var(--line-strong); border-radius: 20px; box-shadow: 0 30px 100px rgba(0,0,0,.5); }
	.modal-header { display: flex; align-items: flex-start; justify-content: space-between; padding: 24px 26px 18px; border-bottom: 1px solid var(--line); }
	.modal-header h2 { margin: 8px 0 0; font: 600 22px/1.1 var(--display); letter-spacing: -.025em; }
	.modal-content { padding: 22px 26px 26px; }
	.icon-button { width: 34px; height: 34px; display: grid; place-items: center; border: 1px solid var(--line); border-radius: 10px; background: rgba(255,255,255,.04); color: var(--muted); font-size: 22px; cursor: pointer; }
	.icon-button:hover { color: var(--text); border-color: var(--line-strong); }
</style>
