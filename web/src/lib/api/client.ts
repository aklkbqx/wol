import type { Bootstrap, Device, Group, Site, WakeAttempt } from '$lib/types/domain';

type APIResponse<T> = {
	success: boolean;
	data?: T;
	message?: string;
};

const basePath = '/api/v1';

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	const response = await fetch(`${basePath}${path}`, {
		headers: {
			'Content-Type': 'application/json',
			...(init.headers ?? {})
		},
		...init
	});
	const payload = (await response.json()) as APIResponse<T>;
	if (!response.ok || !payload.success) {
		throw new Error(payload.message || `Request failed with status ${response.status}`);
	}
	return payload.data as T;
}

export const api = {
	bootstrap: () => request<Bootstrap>('/bootstrap'),
	sites: () => request<Site[]>('/sites'),
	createSite: (input: Partial<Site>) => request<Site>('/sites', { method: 'POST', body: JSON.stringify(input) }),
	updateSite: (id: string, input: Partial<Site>) => request<Site>(`/sites/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),
	deleteSite: (id: string) => request<{ deleted: boolean }>(`/sites/${id}`, { method: 'DELETE' }),
	devices: () => request<Device[]>('/devices'),
	createDevice: (input: Partial<Device>) => request<Device>('/devices', { method: 'POST', body: JSON.stringify(input) }),
	updateDevice: (id: string, input: Partial<Device>) => request<Device>(`/devices/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),
	deleteDevice: (id: string) => request<{ deleted: boolean }>(`/devices/${id}`, { method: 'DELETE' }),
	groups: () => request<Group[]>('/groups'),
	createGroup: (input: Partial<Group>) => request<Group>('/groups', { method: 'POST', body: JSON.stringify(input) }),
	updateGroup: (id: string, input: Partial<Group>) => request<Group>(`/groups/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),
	deleteGroup: (id: string) => request<{ deleted: boolean }>(`/groups/${id}`, { method: 'DELETE' }),
	wakeDevice: (id: string, input: { verify?: boolean; timeoutSeconds?: number; repeat?: number }) => request<WakeAttempt>(`/devices/${id}/wake`, { method: 'POST', body: JSON.stringify(input) }),
	wakeGroup: (id: string, input: { verify?: boolean; timeoutSeconds?: number; repeat?: number }) => request<{ attempts: WakeAttempt[] }>(`/groups/${id}/wake`, { method: 'POST', body: JSON.stringify(input) }),
	history: (limit = 100) => request<WakeAttempt[]>(`/history?limit=${limit}`),
	importData: (input: unknown) => request<{ imported: boolean }>('/import', { method: 'POST', body: JSON.stringify(input) }),
	exportURL: `${basePath}/export`,
	health: () => fetch('/healthz').then((response) => response.ok)
};

export function subscribeToEvents(onAttempt: (attempt: WakeAttempt) => void): () => void {
	const source = new EventSource(`${basePath}/events`);
	const listener = (event: MessageEvent<string>) => {
		onAttempt(JSON.parse(event.data) as WakeAttempt);
	};
	source.addEventListener('wake', listener);
	return () => source.close();
}
