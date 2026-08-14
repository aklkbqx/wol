<script lang="ts">
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { appState, refreshAll } from '$lib/state/app.svelte';
	import { isWakeFailure, wakeLabel, wakeTone } from '$lib/utils/wake';

	let filter = $state('all');
	let history = $derived(appState.history.filter((attempt) => filter === 'all' || (filter === 'failed' && isWakeFailure(attempt)) || (filter === 'verified' && attempt.verificationStatus === 'reachable')));

	function formatTime(value: string) { return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)); }
</script>

<svelte:head><title>History · WOL</title></svelte:head>

<div class="page-heading"><div><span class="eyebrow">Operational evidence</span><h1>History</h1><p>Packet delivery and reachability are separate signals. Use this timeline to understand both.</p></div><div class="heading-actions"><button class="button button-ghost" onclick={refreshAll}>Refresh history</button></div></div>

<section class="panel">
	<div class="toolbar"><div class="filter-tabs" role="tablist"><button class:active={filter === 'all'} onclick={() => (filter = 'all')}>All</button><button class:active={filter === 'verified'} onclick={() => (filter = 'verified')}>Verified</button><button class:active={filter === 'failed'} onclick={() => (filter = 'failed')}>Needs attention</button></div><span class="muted">{history.length} events</span></div>
	{#if history.length}
		<div class="table-wrap"><table><thead><tr><th>Target</th><th>Signal trace</th><th>Route</th><th>Packets</th><th>Time</th></tr></thead><tbody>{#each history as attempt}<tr><td><div class="primary-cell">{attempt.targetName}<span class="secondary-cell mono">{attempt.macAddress}</span></div></td><td><div class="trace-inline"><span class="trace-node sent">●</span><span>Packet {attempt.packetStatus === 'sent' ? 'sent' : 'failed'}</span><span class="trace-arrow">→</span><span class:reachable={attempt.verificationStatus === 'reachable'} class="trace-node">●</span><span>{attempt.verificationStatus === 'not_requested' ? 'Not checked' : attempt.verificationStatus}</span></div>{#if attempt.message}<span class="secondary-cell">{attempt.message}</span>{/if}</td><td><span class="code-pill">{attempt.destination}:{attempt.port}</span></td><td class="mono">{attempt.packets}</td><td><div class="time-cell"><StatusBadge value={wakeLabel(attempt)} tone={wakeTone(attempt)} /><span>{formatTime(attempt.createdAt)}</span></div></td></tr>{/each}</tbody></table></div>
	{:else}
		<div class="panel-body"><EmptyState title={filter === 'all' ? 'No wake history' : 'No matching events'} message={filter === 'all' ? 'Wake a device or group to create the first signal trace.' : 'Try another history filter.'} /></div>
	{/if}
</section>

<style>
	.filter-tabs { display: flex; gap: 3px; padding: 3px; border: 1px solid var(--line); border-radius: 9px; background: rgba(5,12,18,.35); }
	.filter-tabs button { min-height: 29px; padding: 0 10px; border: 0; border-radius: 6px; color: var(--muted); background: transparent; cursor: pointer; font-size: 11px; }
	.filter-tabs button.active { color: var(--text); background: var(--surface-bright); }
	.trace-inline { display: flex; align-items: center; gap: 7px; color: var(--muted-strong); font: 11px var(--mono); }
	.trace-node { color: var(--amber); font-size: 10px; }
	.trace-node.sent { color: var(--signal); }
	.trace-node.reachable { color: var(--signal); }
	.trace-arrow { color: #536875; }
	.time-cell { display: grid; justify-items: start; gap: 7px; color: var(--muted); font: 11px var(--mono); }
	@media (max-width: 760px) { .trace-inline { min-width: 180px; flex-wrap: wrap; } }
</style>
