//go:build windows || plan9 || js || wasip1

package localremote

// Be conservative on platforms without a portable signal-0 probe. Graceful
// Close still removes resources, while automatic orphan cleanup is skipped.
func processAlive(int) bool { return true }
