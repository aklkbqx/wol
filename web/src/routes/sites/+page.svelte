<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { api } from '$lib/api/client';
	import { appState, refreshAll } from '$lib/state/app.svelte';
	import type { Site } from '$lib/types/domain';

	let showForm = $state(false);
	let editing = $state<Site | null>(null);
	let busy = $state(false);
	let error = $state('');
	let form = $state({ name: '', broadcastAddress: '', defaultPort: 9, defaultInterface: '' });
	let sites = $derived(appState.bootstrap?.sites ?? []);
	let devices = $derived(appState.bootstrap?.devices ?? []);

	function resetForm() { form = { name: '', broadcastAddress: '', defaultPort: 9, defaultInterface: '' }; editing = null; error = ''; }
	function openCreate() { resetForm(); showForm = true; }
	function openEdit(site: Site) { editing = site; form = { name: site.name, broadcastAddress: site.broadcastAddress, defaultPort: site.defaultPort, defaultInterface: site.defaultInterface }; error = ''; showForm = true; }
	async function save() {
		busy = true; error = '';
		try { if (editing) await api.updateSite(editing.id, form); else await api.createSite(form); await refreshAll(); showForm = false; }
		catch (caught) { error = caught instanceof Error ? caught.message : 'Could not save site'; }
		finally { busy = false; }
	}
	async function remove(site: Site) {
		if (!confirm(`Delete ${site.name}? Devices will keep their records.`)) return;
		try { await api.deleteSite(site.id); await refreshAll(); } catch (caught) { error = caught instanceof Error ? caught.message : 'Could not delete site'; }
	}
</script>

<svelte:head><title>Sites · WOL</title></svelte:head>

<div class="page-heading"><div><span class="eyebrow">Network defaults</span><h1>Sites</h1><p>Define where packets travel once, then let devices inherit the right broadcast route.</p></div><div class="heading-actions"><button class="button button-primary" onclick={openCreate}>＋ Add site</button></div></div>

{#if error}<div class="notice" role="alert"><strong>Action failed.</strong><span>{error}</span></div>{/if}

{#if sites.length}
	<div class="site-grid">
		{#each sites as site}
			<article class="panel site-card"><div class="site-accent"></div><div class="site-card-top"><div><span class="eyebrow">Network site</span><h2>{site.name}</h2></div><button class="text-button" aria-label={`Edit ${site.name}`} onclick={() => openEdit(site)}>•••</button></div><div class="route"><span>Broadcast route</span><strong class="mono">{site.broadcastAddress || '255.255.255.255'}:{site.defaultPort || 9}</strong></div><div class="site-meta"><span>{devices.filter((device) => device.siteId === site.id).length} devices</span><span class="mono">{site.defaultInterface || 'auto interface'}</span></div><div class="card-actions"><button class="button button-ghost" onclick={() => openEdit(site)}>Edit defaults</button><button class="text-button danger" onclick={() => remove(site)}>Delete</button></div></article>
		{/each}
	</div>
{:else}
	<EmptyState title="No network sites" message="Create a site for each LAN or VLAN. Devices can inherit its broadcast address, port and interface." actionLabel="Add site" onaction={openCreate} />
{/if}

<Modal open={showForm} title={editing ? `Edit ${editing.name}` : 'Add site'} eyebrow="Network configuration" onclose={() => (showForm = false)}>
	<div class="form-grid"><div class="form-field"><label for="site-name">Name</label><input id="site-name" placeholder="e.g. home" bind:value={form.name} /></div><div class="form-field"><label for="site-broadcast">Broadcast address</label><input id="site-broadcast" class="mono" placeholder="192.168.1.255" bind:value={form.broadcastAddress} /><small>Leave empty only if you intentionally use the global broadcast.</small></div><div class="form-field"><label for="site-port">Default UDP port</label><input id="site-port" type="number" min="1" max="65535" bind:value={form.defaultPort} /></div><div class="form-field"><label for="site-interface">Default interface</label><input id="site-interface" class="mono" placeholder="eth0" bind:value={form.defaultInterface} /></div></div>
	{#if error}<p class="error-text" role="alert">{error}</p>{/if}<div class="modal-actions"><button class="button button-ghost" onclick={() => (showForm = false)} disabled={busy}>Cancel</button><button class="button button-primary" onclick={save} disabled={busy}>{busy ? 'Saving…' : editing ? 'Save changes' : 'Add site'}</button></div>
</Modal>

<style>
	.site-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
	.site-card { min-height: 252px; padding: 21px; overflow: hidden; position: relative; }
	.site-accent { position: absolute; top: 0; left: 22px; right: 22px; height: 2px; background: linear-gradient(90deg, var(--signal), transparent); box-shadow: 0 0 18px rgba(56,217,169,.5); }
	.site-card-top { display: flex; justify-content: space-between; gap: 12px; }
	.site-card h2 { margin: 8px 0 0; font: 600 22px var(--display); }
	.route { display: grid; gap: 7px; margin: 34px 0 22px; }
	.route span { color: var(--muted); font: 10px var(--mono); letter-spacing: .1em; text-transform: uppercase; }
	.route strong { color: var(--signal); font-size: 13px; }
	.site-meta { display: flex; justify-content: space-between; gap: 10px; padding-top: 14px; border-top: 1px solid var(--line); color: var(--muted); font-size: 11px; }
	.card-actions { display: flex; align-items: center; gap: 12px; margin-top: 17px; }
	.card-actions .button { flex: 1; }
	@media (max-width: 1100px) { .site-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 640px) { .site-grid { grid-template-columns: 1fr; } }
</style>
