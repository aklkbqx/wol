<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import WakeDialog from '$lib/components/WakeDialog.svelte';
	import { api } from '$lib/api/client';
	import { appState, refreshAll } from '$lib/state/app.svelte';
	import type { Device, Group } from '$lib/types/domain';

	let showForm = $state(false);
	let editing = $state<Group | null>(null);
	let busy = $state(false);
	let error = $state('');
	let form = $state({ name: '', description: '', deviceIds: [] as string[] });
	let wakeTarget = $state<{ kind: 'group'; value: Group } | null>(null);
	let groups = $derived(appState.bootstrap?.groups ?? []);
	let devices = $derived(appState.bootstrap?.devices ?? []);

	function resetForm() { form = { name: '', description: '', deviceIds: [] }; editing = null; error = ''; }
	function openCreate() { resetForm(); showForm = true; }
	function openEdit(group: Group) { editing = group; form = { name: group.name, description: group.description, deviceIds: [...group.deviceIds] }; error = ''; showForm = true; }
	function toggleDevice(id: string) { form.deviceIds = form.deviceIds.includes(id) ? form.deviceIds.filter((item) => item !== id) : [...form.deviceIds, id]; }
	function deviceName(id: string) { return devices.find((device) => device.id === id)?.name ?? 'Unknown device'; }
	async function save() {
		busy = true; error = '';
		try { if (editing) await api.updateGroup(editing.id, form); else await api.createGroup(form); await refreshAll(); showForm = false; }
		catch (caught) { error = caught instanceof Error ? caught.message : 'Could not save group'; }
		finally { busy = false; }
	}
	async function remove(group: Group) {
		if (!confirm(`Delete ${group.name}? Devices will remain.`)) return;
		try { await api.deleteGroup(group.id); await refreshAll(); } catch (caught) { error = caught instanceof Error ? caught.message : 'Could not delete group'; }
	}
</script>

<svelte:head><title>Groups · WOL</title></svelte:head>

<div class="page-heading"><div><span class="eyebrow">Coordinated commands</span><h1>Groups</h1><p>Bundle targets into an intentional wake sequence instead of repeating the same command one device at a time.</p></div><div class="heading-actions"><button class="button button-primary" onclick={openCreate}>＋ Add group</button></div></div>

{#if error}<div class="notice" role="alert"><strong>Action failed.</strong><span>{error}</span></div>{/if}

{#if groups.length}
	<div class="group-grid">{#each groups as group}<article class="panel group-card"><div class="group-top"><div class="group-symbol" aria-hidden="true">◇</div><div class="group-actions"><button class="text-button" onclick={() => openEdit(group)}>Edit</button><button class="text-button danger" onclick={() => remove(group)}>Delete</button></div></div><h2>{group.name}</h2><p>{group.description || 'No description yet.'}</p><div class="member-list">{#if group.deviceIds.length}{#each group.deviceIds.slice(0, 4) as id}<span class="member-chip">{deviceName(id)}</span>{/each}{#if group.deviceIds.length > 4}<span class="member-chip more">+{group.deviceIds.length - 4} more</span>{/if}{:else}<span class="muted">No devices selected</span>{/if}</div><div class="group-footer"><span>{group.deviceIds.length} target{group.deviceIds.length === 1 ? '' : 's'}</span><button class="button button-amber" onclick={() => (wakeTarget = { kind: 'group', value: group })} disabled={!group.deviceIds.length}>Wake group ↗</button></div></article>{/each}</div>
{:else}
	<EmptyState title="No groups yet" message="Create a group for a repeatable command such as Home Lab, Office Servers or Database Stack." actionLabel="Add group" onaction={openCreate} />
{/if}

<Modal open={showForm} title={editing ? `Edit ${editing.name}` : 'Add group'} eyebrow="Command grouping" onclose={() => (showForm = false)}>
	<div class="form-grid"><div class="form-field full"><label for="group-name">Name</label><input id="group-name" placeholder="e.g. home-lab" bind:value={form.name} /></div><div class="form-field full"><label for="group-description">Description</label><textarea id="group-description" placeholder="What is this group for?" bind:value={form.description}></textarea></div><div class="form-field full"><span class="form-label">Devices · {form.deviceIds.length} selected</span><div class="device-picker">{#if devices.length}{#each devices as device}<label class="device-option"><input type="checkbox" checked={form.deviceIds.includes(device.id)} onchange={() => toggleDevice(device.id)} /><span><strong>{device.name}</strong><small class="mono">{device.macAddress}</small></span></label>{/each}{:else}<span class="muted">Add devices before creating a group.</span>{/if}</div></div></div>
	{#if error}<p class="error-text" role="alert">{error}</p>{/if}<div class="modal-actions"><button class="button button-ghost" onclick={() => (showForm = false)} disabled={busy}>Cancel</button><button class="button button-primary" onclick={save} disabled={busy || !form.name.trim()}>{busy ? 'Saving…' : editing ? 'Save changes' : 'Create group'}</button></div>
</Modal>

<WakeDialog target={wakeTarget} onclose={() => (wakeTarget = null)} />

<style>
	.group-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
	.group-card { min-height: 250px; display: flex; flex-direction: column; padding: 21px; }
	.group-top { display: flex; align-items: center; justify-content: space-between; }
	.group-symbol { width: 38px; height: 38px; display: grid; place-items: center; color: var(--signal); border: 1px solid rgba(56,217,169,.25); border-radius: 12px; background: rgba(56,217,169,.08); font: 23px var(--mono); }
	.group-actions { display: flex; gap: 3px; }
	.group-card h2 { margin: 22px 0 6px; font: 600 21px var(--display); }
	.group-card p { min-height: 38px; margin: 0; color: var(--muted); font-size: 12px; line-height: 1.6; }
	.member-list { display: flex; flex-wrap: wrap; gap: 6px; margin: 19px 0; }
	.member-chip { padding: 5px 8px; border: 1px solid var(--line); border-radius: 7px; color: var(--muted-strong); background: rgba(255,255,255,.04); font-size: 11px; }
	.member-chip.more { color: var(--signal); }
	.group-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: auto; padding-top: 15px; border-top: 1px solid var(--line); color: var(--muted); font: 11px var(--mono); }
	.group-footer .button { min-height: 34px; padding: 0 11px; font-size: 11px; }
	.device-picker { display: grid; max-height: 240px; overflow: auto; gap: 7px; padding: 8px; border: 1px solid var(--line); border-radius: 11px; background: rgba(5,12,18,.32); }
	.device-option { display: flex; align-items: center; gap: 10px; padding: 10px; border-radius: 8px; cursor: pointer; }
	.device-option:hover { background: rgba(255,255,255,.04); }
	.device-option input { accent-color: var(--signal); }
	.device-option span { display: grid; gap: 3px; }
	.device-option strong { color: var(--text); font-size: 12px; }
	.device-option small { color: var(--muted); font-size: 10px; }
	@media (max-width: 1100px) { .group-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 640px) { .group-grid { grid-template-columns: 1fr; } }
</style>
