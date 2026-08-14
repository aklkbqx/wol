<script lang="ts">
	let { title, message, actionLabel = '', onaction = () => {} }: { title: string; message: string; actionLabel?: string; onaction?: () => void } = $props();
</script>

<div class="empty-state">
	<div class="empty-orbit" aria-hidden="true"><span></span></div>
	<h3>{title}</h3>
	<p>{message}</p>
	{#if actionLabel}<button class="button button-primary" onclick={onaction}>{actionLabel}</button>{/if}
</div>

<style>
	.empty-state { display: grid; justify-items: center; gap: 10px; padding: 64px 24px; text-align: center; border: 1px dashed rgba(145,164,178,.25); border-radius: 16px; background: rgba(255,255,255,.018); }
	.empty-orbit { width: 50px; height: 50px; display: grid; place-items: center; border: 1px solid rgba(56,217,169,.35); border-radius: 50%; background: rgba(56,217,169,.06); }
	.empty-orbit span { width: 8px; height: 8px; border-radius: 50%; background: var(--signal); box-shadow: 0 0 20px var(--signal); }
	.empty-state h3 { margin: 6px 0 0; font: 600 18px var(--display); }
	.empty-state p { max-width: 390px; margin: 0; color: var(--muted); font-size: 13px; line-height: 1.6; }
</style>
