<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import WakeDialog from '$lib/components/WakeDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { api } from '$lib/api/client';
	import { appState, refreshAll } from '$lib/state/app.svelte';
	import type { Device } from '$lib/types/domain';

	let search = $state('');
	let showForm = $state(false);
	let editing = $state<Device | null>(null);
	let busy = $state(false);
	let error = $state('');
	let wakeTarget = $state<{ kind: 'device'; value: Device } | null>(null);
	let form = $state({ name: '', macAddress: '', ipAddress: '', broadcastAddress: '', port: 0, interface: '', siteId: '', deviceType: 'server', verifyPort: 0, description: '', enabled: true });

	let devices = $derived(appState.bootstrap?.devices ?? []);
	let sites = $derived(appState.bootstrap?.sites ?? []);
	let filtered = $derived(devices.filter((device) => `${device.name} ${device.macAddress} ${device.ipAddress} ${device.deviceType}`.toLowerCase().includes(search.toLowerCase())));

	function siteName(siteId: string) { return appState.bootstrap?.sites.find((site) => site.id === siteId)?.name ?? 'Unassigned'; }
	function resetForm() { form = { name: '', macAddress: '', ipAddress: '', broadcastAddress: '', port: 0, interface: '', siteId: '', deviceType: 'server', verifyPort: 0, description: '', enabled: true }; editing = null; error = ''; }
	function openCreate() { resetForm(); showForm = true; }
	function openEdit(device: Device) { editing = device; form = { name: device.name, macAddress: device.macAddress, ipAddress: device.ipAddress, broadcastAddress: device.broadcastAddress, port: device.port, interface: device.interface, siteId: device.siteId, deviceType: device.deviceType, verifyPort: device.verifyPort, description: device.description, enabled: device.enabled }; error = ''; showForm = true; }
	function closeForm() { if (!busy) showForm = false; }

	async function save() {
		busy = true; error = '';
		try {
			if (editing) await api.updateDevice(editing.id, form);
			else await api.createDevice(form);
			await refreshAll(); showForm = false;
		} catch (caught) { error = caught instanceof Error ? caught.message : 'Could not save device'; }
		finally { busy = false; }
	}

	async function remove(device: Device) {
		if (!confirm(`Delete ${device.name}? Wake history will remain.`)) return;
		try { await api.deleteDevice(device.id); await refreshAll(); }
		catch (caught) { error = caught instanceof Error ? caught.message : 'Could not delete device'; }
	}
</script>

<svelte:head><title>Devices · WOL</title></svelte:head>

<div class="page-heading">
	<div><span class="eyebrow">Target inventory</span><h1>Devices</h1><p>Keep each MAC address close to the network route and verification rule that makes it useful.</p></div>
	<div class="heading-actions"><button class="button button-primary" onclick={openCreate}>＋ Add device</button></div>
</div>

{#if error}<div class="notice" role="alert"><strong>Action failed.</strong><span>{error}</span></div>{/if}

<section class="panel">
	<div class="toolbar"><div class="search-box"><span>⌕</span><input aria-label="Search devices" placeholder="Search name, MAC, IP or type" bind:value={search} /></div><span class="muted">{filtered.length} of {devices.length} devices</span></div>
	{#if filtered.length}
		<div class="table-wrap"><table><thead><tr><th>Device</th><th>Network identity</th><th>Site</th><th>State</th><th>Last route</th><th></th></tr></thead><tbody>
			{#each filtered as device}
				<tr>
					<td><div class="primary-cell">{device.name}<span class="secondary-cell">{device.deviceType}{device.description ? ` · ${device.description}` : ''}</span></div></td>
					<td><div class="primary-cell mono">{device.macAddress}<span class="secondary-cell">{device.ipAddress || 'IP not set'}</span></div></td>
					<td>{siteName(device.siteId)}</td>
					<td><StatusBadge value={device.enabled ? 'Enabled' : 'Disabled'} tone={device.enabled ? 'success' : 'neutral'} /></td>
					<td><span class="code-pill">{device.broadcastAddress || 'Auto'}:{device.port || 9}</span></td>
					<td><div class="row-actions"><button class="text-button" onclick={() => (wakeTarget = { kind: 'device', value: device })}>Wake</button><button class="text-button" onclick={() => openEdit(device)}>Edit</button><button class="text-button danger" onclick={() => remove(device)}>Delete</button></div></td>
				</tr>
			{/each}
		</tbody></table></div>
	{:else}
		<div class="panel-body"><EmptyState title={search ? 'No matching devices' : 'No devices yet'} message={search ? 'Try a different name, MAC address or IP.' : 'Add your first target to make the wake center useful.'} actionLabel={search ? '' : 'Add device'} onaction={openCreate} /></div>
	{/if}
</section>

<Modal open={showForm} title={editing ? `Edit ${editing.name}` : 'Add device'} eyebrow="Device inventory" onclose={closeForm}>
	<div class="form-grid">
		<div class="form-field"><label for="device-name">Name</label><input id="device-name" placeholder="e.g. home-nas" bind:value={form.name} /></div>
		<div class="form-field"><label for="device-type">Type</label><select id="device-type" bind:value={form.deviceType}><option value="server">Server</option><option value="nas">NAS</option><option value="desktop">Desktop</option><option value="router">Router</option><option value="other">Other</option></select></div>
		<div class="form-field"><label for="mac">MAC address</label><input id="mac" class="mono" placeholder="AA:BB:CC:DD:EE:FF" bind:value={form.macAddress} /><small>Colons, dashes and dotted notation are accepted.</small></div>
		<div class="form-field"><label for="ip">IP address</label><input id="ip" class="mono" placeholder="192.168.1.20" bind:value={form.ipAddress} /></div>
		<div class="form-field"><label for="site">Site</label><select id="site" bind:value={form.siteId}><option value="">No site</option>{#each sites as site}<option value={site.id}>{site.name}</option>{/each}</select><small>Site defaults fill broadcast, port and interface.</small></div>
		<div class="form-field"><label for="broadcast">Broadcast address</label><input id="broadcast" class="mono" placeholder="192.168.1.255" bind:value={form.broadcastAddress} /></div>
		<div class="form-field"><label for="port">UDP port</label><input id="port" type="number" min="0" max="65535" placeholder="9" bind:value={form.port} /></div>
		<div class="form-field"><label for="interface">Interface</label><input id="interface" class="mono" placeholder="eth0" bind:value={form.interface} /></div>
		<div class="form-field"><label for="verify-port">Verification TCP port</label><input id="verify-port" type="number" min="0" max="65535" placeholder="22" bind:value={form.verifyPort} /><small>Leave empty to send without checking reachability.</small></div>
		<div class="form-field full"><label for="description">Description</label><textarea id="description" placeholder="Where is this device? What should it be used for?" bind:value={form.description}></textarea></div>
		<label class="checkbox-row full"><input type="checkbox" bind:checked={form.enabled} /> Include this device in group commands</label>
	</div>
	{#if error}<p class="error-text" role="alert">{error}</p>{/if}
	<div class="modal-actions"><button class="button button-ghost" onclick={closeForm} disabled={busy}>Cancel</button><button class="button button-primary" onclick={save} disabled={busy}>{busy ? 'Saving…' : editing ? 'Save changes' : 'Add device'}</button></div>
</Modal>

<WakeDialog target={wakeTarget} onclose={() => (wakeTarget = null)} />
