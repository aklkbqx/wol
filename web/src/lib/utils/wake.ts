import type { WakeAttempt } from '$lib/types/domain';

export function isWakeFailure(attempt: WakeAttempt): boolean {
	return attempt.packetStatus === 'failed' || attempt.verificationStatus === 'timeout';
}

export function wakeLabel(attempt: WakeAttempt): string {
	if (attempt.packetStatus === 'failed') return 'Failed';
	if (attempt.verificationStatus === 'reachable') return 'Reachable';
	if (attempt.verificationStatus === 'timeout') return 'Timed out';
	return 'Packet sent';
}

export function wakeTone(attempt: WakeAttempt): 'success' | 'warning' | 'danger' {
	return isWakeFailure(attempt) ? 'danger' : attempt.verificationStatus === 'reachable' ? 'success' : 'warning';
}
