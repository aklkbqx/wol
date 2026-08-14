<script lang="ts">
	import { appState, refreshAll } from '$lib/state/app.svelte';
	import type { Device, Group } from '$lib/types/domain';
	import StatCard from '$lib/components/StatCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import WakeDialog from '$lib/components/WakeDialog.svelte';

	let wakeTarget = $state<{ kind: 'device'; value: Device } | { kind: 'group'; value: Group } | null>(null);
	let devices = $derived(appState.bootstrap?.devices ?? []);
	let sites = $derived(appState.bootstrap?.sites ?? []);
	let groups = $derived(appState.bootstrap?.groups ?? []);
	let recent = $derived(appState.history.slice(0, 5));
	let enabledCount = $derived(devices.filter((device) => device.enabled).length);
	let reachableCount = $derived(appState.history.filter((item) => item.verificationStatus === 'reachable').length);
	let failedCount = $derived(appState.history.filter((item) => item.packetStatus === 'failed' || item.verificationStatus === 'timeout').length);

	function siteName(siteId: string) {
		return sites.find((site) => site.id === siteId)?.name ?? 'Unassigned site';
	}

	function formatTime(value: string) {
		if (!value) return 'No activity yet';
		return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', month: 'short', day: 'numeric' }).format(new Date(value));
	}
</script>

<svelte:head><title>Wake center · WOL</title></svelte:head>

<div class="page-heading">
	<div>
		<span class="eyebrow">Local network operations</span>
		<h1>Wake center</h1>
		<p>See the signal path, choose a target and send a wake packet without losing the details that explain what happened next.</p>
	</div>
	<div class="heading-actions">
		<a class="button button-ghost" href="/devices">Manage devices</a>
		<a class="button button-amber" href="/groups">Wake a group <span aria-hidden="true">↗</span></a>
	</div>
</div>

<div class="stats-grid">
	<StatCard label="Configured devices" value={devices.length} detail={`${enabledCount} enabled`} />
	<StatCard label="Network sites" value={sites.length} detail={sites.length ? 'Ready for routing' : 'Add your first site'} accent="amber" />
	<StatCard label="Verified wakes" value={reachableCount} detail="From recent history" />
	<StatCard label="Needs attention" value={failedCount} detail={failedCount ? 'Review recent attempts' : 'No recent failures'} accent={failedCount ? 'danger' : 'signal'} />
</div>

<div class="content-grid">
	<section class="panel">
		<div class="panel-header">
			<div><span class="eyebrow">Recent transmissions</span><h2>Signal trace</h2><p>Each wake is recorded independently from verification.</p></div>
			<button class="button button-ghost" onclick={refreshAll}>Refresh</button>
		</div>
		<div class="panel-body">
			{#if recent.length}
				<div class="trace">
					{#each recent as attempt}
						<div class:failed={attempt.packetStatus === 'failed' || attempt.verificationStatus === 'timeout'} class="trace-step">
							<div class="trace-marker"></div>
							<div class="trace-copy"><strong>{attempt.targetName} · {attempt.verificationStatus === 'reachable' ? 'Device reachable' : attempt.packetStatus === 'sent' ? 'Packet sent' : 'Transmission failed'}</strong><span>{formatTime(attempt.createdAt)} · {attempt.destination}:{attempt.port} · {attempt.packets} packets</span></div>
						</div>
					{/each}
				</div>
			{:else}
				<div class="empty-state"><div class="empty-orbit"><span></span></div><h3>No signal traces yet</h3><p>Your first wake command will appear here with its destination and verification outcome.</p><a class="button button-primary" href="/devices">Choose a device</a></div>
			{/if}
		</div>
	</section>

	<div class="stack">
		<section class="panel">
			<div class="panel-header"><div><span class="eyebrow">Inventory</span><h2>Sites</h2></div><a class="text-button" href="/sites">View all →</a></div>
			<div class="panel-body">
				{#if sites.length}
					<div class="list">{#each sites.slice(0, 4) as site}<div class="list-item"><div class="list-item-main"><span class="list-item-title">{site.name}</span><span class="list-item-meta mono">{site.broadcastAddress || 'No broadcast configured'} · {site.defaultPort || 9}</span></div><span class="code-pill">{devices.filter((device) => device.siteId === site.id).length} devices</span></div>{/each}</div>
				{:else}
					<p class="muted">Add a site to keep network defaults out of every device form.</p>
				{/if}
			</div>
		</section>

		<section class="panel">
			<div class="panel-header"><div><span class="eyebrow">Quick targets</span><h2>Enabled devices</h2></div><a class="text-button" href="/devices">View all →</a></div>
			<div class="panel-body">
				{#if devices.filter((device) => device.enabled).length}
					<div class="list">{#each devices.filter((device) => device.enabled).slice(0, 4) as device}<div class="list-item"><div class="list-item-main"><span class="list-item-title">{device.name}</span><span class="list-item-meta">{siteName(device.siteId)} · <span class="mono">{device.macAddress}</span></span></div><button class="text-button" onclick={() => (wakeTarget = { kind: 'device', value: device })}>Wake</button></div>{/each}</div>
				{:else}
					<p class="muted">No enabled devices yet.</p>
				{/if}
			</div>
		</section>
	</div>
</div>

<WakeDialog target={wakeTarget} onclose={() => (wakeTarget = null)} />
