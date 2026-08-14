<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { refreshAll, startEvents, stopEvents } from '$lib/state/app.svelte';
	import '../app.css';

	let { children }: { children: Snippet } = $props();

	onMount(() => {
		refreshAll();
		startEvents();
		return stopEvents;
	});
</script>

<AppShell>
	{@render children()}
</AppShell>
