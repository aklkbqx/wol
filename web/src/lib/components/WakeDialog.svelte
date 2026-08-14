<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { api } from '$lib/api/client';
	import { refreshAll } from '$lib/state/app.svelte';
	import type { Device, Group, WakeAttempt } from '$lib/types/domain';

	type Target = { kind: 'device'; value: Device } | { kind: 'group'; value: Group } | null;
	let { target = null, onclose, oncomplete }: { target?: Target; onclose: () => void; oncomplete?: () => void } = $props();
	let busy = $state(false);
	let verify = $state(true);
	let timeoutSeconds = $state(30);
	let result = $state<WakeAttempt[] | null>(null);
	let error = $state('');

	const isOpen = $derived(target !== null);
	const title = $derived(target?.kind === 'group' ? `Wake ${target.value.name}` : `Wake ${target?.value.name ?? 'device'}`);

	async function submit() {
		if (!target) return;
		busy = true;
		error = '';
		result = null;
		try {
			if (target.kind === 'group') {
				const response = await api.wakeGroup(target.value.id, { verify, timeoutSeconds });
				result = response.attempts;
			} else {
				result = [await api.wakeDevice(target.value.id, { verify, timeoutSeconds })];
			}
			await refreshAll();
			oncomplete?.();
		} catch (caught) {
			error = caught instanceof Error ? caught.message : 'Wake request failed';
		} finally {
			busy = false;
		}
	}

	function close() {
		if (!busy) {
			result = null;
			error = '';
			onclose();
		}
	}
</script>

<Modal open={isOpen} title={title} eyebrow={target?.kind === 'group' ? 'Group command' : 'Wake command'} onclose={close}>
		{#if result}
			<div class="result-block">
				<div class="result-heading"><span class="signal-ring">✓</span><div><strong>Command complete</strong><span>{result.length} target{result.length === 1 ? '' : 's'} processed</span></div></div>
				<div class="result-list">
					{#each result as attempt}
						<div class="result-row">
							<div><strong>{attempt.targetName}</strong><span class="mono">{attempt.destination}:{attempt.port}</span></div>
							<StatusBadge value={attempt.verificationStatus === 'reachable' ? 'Reachable' : attempt.packetStatus === 'sent' ? 'Packet sent' : 'Failed'} tone={attempt.verificationStatus === 'reachable' || attempt.packetStatus === 'sent' ? 'success' : 'danger'} />
						</div>
					{/each}
				</div>
				{#if result.some((attempt) => attempt.verificationStatus === 'timeout')}
					<div class="notice"><strong>Packet sent.</strong><span>One or more devices did not respond before the verification timeout.</span></div>
				{/if}
				<div class="modal-actions"><button class="button button-primary" onclick={close}>Done</button></div>
			</div>
		{:else}
			<div class="command-preview">
				<div class="preview-line"><span class="preview-label">Target</span><strong>{target?.value.name}</strong></div>
				<div class="preview-line"><span class="preview-label">Mode</span><span>{target?.kind === 'group' ? 'Sequential group wake' : 'Single device'}</span></div>
			</div>
			<div class="form-grid">
				<label class="checkbox-row full"><input type="checkbox" bind:checked={verify} /> Check whether the device becomes reachable</label>
				{#if verify}
					<div class="form-field"><label for="timeout">Verification timeout</label><input id="timeout" type="number" min="5" max="300" bind:value={timeoutSeconds} /><small>TCP verification uses the port configured on each device.</small></div>
				{/if}
			</div>
			<div class="notice"><strong>Packet status is not power status.</strong><span>WOL can confirm that a packet was sent. Verification is separate and may depend on firewall, boot time and device configuration.</span></div>
			{#if error}<p class="error-text" role="alert">{error}</p>{/if}
			<div class="modal-actions"><button class="button button-ghost" onclick={close} disabled={busy}>Cancel</button><button class="button button-amber" onclick={submit} disabled={busy}>{busy ? 'Sending…' : 'Send wake packet'}</button></div>
		{/if}
</Modal>

<style>
	.command-preview { display: grid; gap: 12px; margin-bottom: 22px; padding: 15px; border: 1px solid rgba(56,217,169,.2); border-radius: 12px; background: rgba(56,217,169,.05); }
	.preview-line { display: flex; justify-content: space-between; gap: 16px; color: var(--muted-strong); font-size: 13px; }
	.preview-line strong { color: var(--text); }
	.preview-label { color: var(--muted); font: 10px var(--mono); letter-spacing: .1em; text-transform: uppercase; }
	.notice { margin-top: 20px; }
	.full { grid-column: 1 / -1; }
	.result-block { display: grid; gap: 18px; }
	.result-heading { display: flex; align-items: center; gap: 12px; padding: 14px; border: 1px solid rgba(56,217,169,.22); border-radius: 12px; background: rgba(56,217,169,.06); }
	.result-heading div { display: grid; gap: 4px; }
	.result-heading strong { color: var(--text); font-size: 14px; }
	.result-heading span:not(.signal-ring) { color: var(--muted); font-size: 12px; }
	.signal-ring { width: 28px; height: 28px; display: grid; place-items: center; color: var(--ink); border-radius: 50%; background: var(--signal); font-weight: 700; }
	.result-list { display: grid; gap: 8px; }
	.result-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; border-bottom: 1px solid var(--line); }
	.result-row > div { display: grid; gap: 4px; }
	.result-row strong { color: var(--text); font-size: 13px; }
	.result-row span { color: var(--muted); font-size: 11px; }
</style>
