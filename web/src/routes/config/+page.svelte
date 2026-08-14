<script lang="ts">
	import { api } from '$lib/api/client';
	import { appState, refreshAll } from '$lib/state/app.svelte';

	let importBusy = $state(false);
	let message = $state('');
	let error = $state('');

	async function importFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		importBusy = true; message = ''; error = '';
		try {
			const content = JSON.parse(await file.text());
			await api.importData(content);
			await refreshAll();
			message = `Imported ${file.name}`;
		} catch (caught) { error = caught instanceof Error ? caught.message : 'Could not import this file'; }
		finally { importBusy = false; input.value = ''; }
	}

	function downloadExport() { window.location.href = api.exportURL; }
</script>

<svelte:head><title>Config & data · WOL</title></svelte:head>

<div class="page-heading"><div><span class="eyebrow">Portable local data</span><h1>Config & data</h1><p>Keep the inventory human-readable, exportable and easy to move between machines without managing a database server.</p></div></div>

{#if message}<div class="success-banner" role="status"><strong>Import complete.</strong><span>{message}</span></div>{/if}
{#if error}<div class="notice" role="alert"><strong>Import failed.</strong><span>{error}</span></div>{/if}

<div class="content-grid">
	<section class="panel">
		<div class="panel-header"><div><span class="eyebrow">Inventory package</span><h2>Import and export</h2><p>Export includes sites, devices and groups. Wake history stays local to this installation.</p></div></div>
		<div class="panel-body config-actions">
			<div class="data-action"><div class="action-icon">↓</div><div><h3>Export inventory</h3><p>Download a JSON file you can version, inspect or move to another WOL installation.</p></div><button class="button button-primary" onclick={downloadExport}>Download export</button></div>
			<div class="data-action"><div class="action-icon">↑</div><div><h3>Import inventory</h3><p>Existing sites, devices and groups are updated by their name or normalized MAC address.</p></div><label class="button button-ghost">{importBusy ? 'Importing…' : 'Choose JSON'}<input type="file" accept="application/json,.json" onchange={importFile} hidden /></label></div>
		</div>
	</section>

	<section class="panel">
		<div class="panel-header"><div><span class="eyebrow">Current store</span><h2>Local database</h2></div></div>
		<div class="panel-body store-summary"><div><span class="summary-label">Sites</span><strong>{appState.bootstrap?.sites.length ?? 0}</strong></div><div><span class="summary-label">Devices</span><strong>{appState.bootstrap?.devices.length ?? 0}</strong></div><div><span class="summary-label">Groups</span><strong>{appState.bootstrap?.groups.length ?? 0}</strong></div><div><span class="summary-label">History events</span><strong>{appState.history.length}</strong></div></div>
	</section>
</div>

<section class="panel guide-panel"><div class="panel-header"><div><span class="eyebrow">Portable workflow</span><h2>Recommended setup</h2></div></div><div class="panel-body"><div class="guide-grid"><div><span class="step-number">01</span><strong>Define a site</strong><p>Set the broadcast address, port and interface once.</p></div><div><span class="step-number">02</span><strong>Add devices</strong><p>Store MAC, IP and verification details close to the target.</p></div><div><span class="step-number">03</span><strong>Export a copy</strong><p>Keep inventory JSON in a private backup or repository.</p></div></div></div></section>

<style>
	.config-actions { display: grid; gap: 15px; }
	.data-action { display: grid; grid-template-columns: 42px minmax(0, 1fr) auto; align-items: center; gap: 14px; padding: 15px 0; border-bottom: 1px solid var(--line); }
	.data-action:last-child { border-bottom: 0; }
	.action-icon { width: 42px; height: 42px; display: grid; place-items: center; color: var(--signal); border: 1px solid rgba(56,217,169,.24); border-radius: 12px; background: rgba(56,217,169,.07); font: 22px var(--mono); }
	.data-action h3 { margin: 0 0 4px; color: var(--text); font: 600 14px var(--display); }
	.data-action p, .guide-grid p { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.55; }
	.success-banner { display: flex; gap: 9px; margin-bottom: 20px; padding: 12px 14px; border: 1px solid rgba(56,217,169,.24); border-radius: 11px; color: var(--signal); background: rgba(56,217,169,.08); font-size: 12px; }
	.success-banner span { color: var(--muted-strong); }
	.store-summary { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
	.store-summary > div { display: grid; gap: 5px; padding: 13px; border: 1px solid var(--line); border-radius: 11px; background: rgba(255,255,255,.025); }
	.summary-label { color: var(--muted); font: 10px var(--mono); text-transform: uppercase; letter-spacing: .08em; }
	.store-summary strong { color: var(--text); font: 600 26px var(--display); }
	.guide-panel { margin-top: 18px; }
	.guide-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; }
	.guide-grid > div { display: grid; gap: 7px; }
	.step-number { color: var(--signal); font: 600 11px var(--mono); letter-spacing: .1em; }
	.guide-grid strong { color: var(--text); font: 600 15px var(--display); }
	@media (max-width: 650px) { .data-action { grid-template-columns: 42px 1fr; } .data-action .button { grid-column: 2; justify-self: start; } .guide-grid { grid-template-columns: 1fr; gap: 16px; } }
</style>
