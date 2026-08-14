<script lang="ts">
	let { label, value, detail, accent = 'signal' }: { label: string; value: string | number; detail: string; accent?: 'signal' | 'amber' | 'danger' } = $props();
</script>

<article class:amber={accent === 'amber'} class:danger={accent === 'danger'} class="stat-card">
	<span class="stat-label">{label}</span>
	<strong>{value}</strong>
	<span class="stat-detail">{detail}</span>
</article>

<style>
	.stat-card { min-height: 134px; display: flex; flex-direction: column; justify-content: space-between; padding: 18px 18px 16px; background: linear-gradient(145deg, rgba(27,42,54,.82), rgba(17,28,39,.8)); border: 1px solid var(--line); border-radius: 16px; position: relative; overflow: hidden; }
	.stat-card::after { content: ''; position: absolute; width: 90px; height: 90px; right: -36px; bottom: -42px; border-radius: 50%; background: rgba(56,217,169,.13); filter: blur(18px); }
	.stat-card.amber::after { background: rgba(255,180,84,.14); }
	.stat-card.danger::after { background: rgba(245,108,108,.15); }
	.stat-label, .stat-detail { color: var(--muted); font: 600 10px var(--mono); letter-spacing: .08em; text-transform: uppercase; }
	.stat-card strong { color: var(--text); font: 600 34px/1 var(--display); letter-spacing: -.04em; }
	.stat-detail { font: 12px var(--body); letter-spacing: 0; text-transform: none; }
</style>
