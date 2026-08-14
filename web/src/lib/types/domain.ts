export type Site = {
	id: string;
	name: string;
	broadcastAddress: string;
	defaultPort: number;
	defaultInterface: string;
	createdAt: string;
	updatedAt: string;
};

export type Device = {
	id: string;
	name: string;
	macAddress: string;
	ipAddress: string;
	broadcastAddress: string;
	port: number;
	interface: string;
	siteId: string;
	deviceType: string;
	verifyPort: number;
	description: string;
	enabled: boolean;
	createdAt: string;
	updatedAt: string;
};

export type Group = {
	id: string;
	name: string;
	description: string;
	deviceIds: string[];
	createdAt: string;
	updatedAt: string;
};

export type WakeAttempt = {
	id: string;
	targetType: string;
	targetId: string;
	targetName: string;
	macAddress: string;
	destination: string;
	port: number;
	packetStatus: 'sent' | 'failed' | string;
	verificationStatus: 'not_requested' | 'checking' | 'reachable' | 'timeout' | 'unavailable' | string;
	message: string;
	packets: number;
	createdAt: string;
};

export type Bootstrap = {
	version: string;
	sites: Site[];
	devices: Device[];
	groups: Group[];
	capabilities: Record<string, boolean>;
};
