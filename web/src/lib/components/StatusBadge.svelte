<script lang="ts">
	let { value, tone = 'neutral' }: { value: string; tone?: 'success' | 'warning' | 'danger' | 'neutral' } = $props();
</script>

<span class:success={tone === 'success'} class:warning={tone === 'warning'} class:danger={tone === 'danger'} class="status-badge"><span class="status-dot"></span>{value}</span>

<style>
	.status-badge { display: inline-flex; align-items: center; gap: 7px; width: fit-content; padding: 6px 9px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); background: rgba(255,255,255,.035); font: 600 10px var(--mono); text-transform: uppercase; letter-spacing: .04em; }
	.status-dot { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
	.success { color: var(--signal); background: rgba(56,217,169,.08); border-color: rgba(56,217,169,.2); }
	.warning { color: var(--amber); background: rgba(255,180,84,.08); border-color: rgba(255,180,84,.2); }
	.danger { color: var(--danger); background: rgba(245,108,108,.08); border-color: rgba(245,108,108,.2); }
</style>
