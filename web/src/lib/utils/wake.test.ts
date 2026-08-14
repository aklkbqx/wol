import { describe, expect, it } from 'vitest';
import { isWakeFailure, wakeLabel, wakeTone } from './wake';

const base = { id: 'wake_1', targetType: 'device', targetId: 'device_1', targetName: 'NAS', macAddress: 'aa:bb:cc:dd:ee:ff', destination: '192.168.1.255', port: 9, message: '', packets: 3, createdAt: '' };

describe('wake display state', () => {
	it('distinguishes a verified target from a sent packet', () => {
		const verified = { ...base, packetStatus: 'sent', verificationStatus: 'reachable' };
		const sent = { ...base, packetStatus: 'sent', verificationStatus: 'not_requested' };

		expect(wakeLabel(verified)).toBe('Reachable');
		expect(wakeTone(verified)).toBe('success');
		expect(wakeLabel(sent)).toBe('Packet sent');
		expect(wakeTone(sent)).toBe('warning');
	});

	it('marks failed packets and verification timeouts as attention states', () => {
		const failed = { ...base, packetStatus: 'failed', verificationStatus: 'not_requested' };
		const timedOut = { ...base, packetStatus: 'sent', verificationStatus: 'timeout' };

		expect(isWakeFailure(failed)).toBe(true);
		expect(isWakeFailure(timedOut)).toBe(true);
		expect(wakeLabel(timedOut)).toBe('Timed out');
		expect(wakeTone(timedOut)).toBe('danger');
	});
});
