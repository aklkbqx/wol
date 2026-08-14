import { api, subscribeToEvents } from '$lib/api/client';
import type { Bootstrap, Device, Group, Site, WakeAttempt } from '$lib/types/domain';

export const appState = $state<{
	bootstrap: Bootstrap | null;
	history: WakeAttempt[];
	loading: boolean;
	error: string;
	connected: boolean;
	lastUpdated: string;
}>({
	bootstrap: null,
	history: [],
	loading: true,
	error: '',
	connected: false,
	lastUpdated: ''
});

let eventStopper: (() => void) | null = null;

export async function refreshAll() {
	appState.loading = true;
	appState.error = '';
	try {
		const [bootstrap, history] = await Promise.all([api.bootstrap(), api.history()]);
		appState.bootstrap = bootstrap;
		appState.history = history;
		appState.connected = true;
		appState.lastUpdated = new Date().toISOString();
	} catch (error) {
		appState.error = error instanceof Error ? error.message : 'Could not connect to WOL server';
		appState.connected = false;
	} finally {
		appState.loading = false;
	}
}

export function startEvents() {
	if (eventStopper) return;
	eventStopper = subscribeToEvents((attempt) => {
		appState.history = [attempt, ...appState.history.filter((item) => item.id !== attempt.id)].slice(0, 100);
		appState.lastUpdated = new Date().toISOString();
	});
}

export function stopEvents() {
	eventStopper?.();
	eventStopper = null;
}

export function devices(): Device[] {
	return appState.bootstrap?.devices ?? [];
}

export function sites(): Site[] {
	return appState.bootstrap?.sites ?? [];
}

export function groups(): Group[] {
	return appState.bootstrap?.groups ?? [];
}
