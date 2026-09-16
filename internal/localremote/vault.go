package localremote

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"
)

const vaultService = "wol.localremote"

// ErrVaultMiss means no sign-in is stored for this machine.
var ErrVaultMiss = errors.New("no saved sign-in")

// Vault stores local-remote passwords in the OS keychain, never in SQLite.
type Vault interface {
	Get(key string) (Credentials, error)
	Put(key string, creds Credentials) error
	Delete(key string) error
}

// VaultKey identifies a saved sign-in by protocol and host, not by inventory path.
func VaultKey(protocol, host string, port int) string {
	return fmt.Sprintf("%s|%s|%d", protocol, host, port)
}

// MemoryVault is an in-process vault for tests.
type MemoryVault struct {
	mu    sync.Mutex
	items map[string]Credentials
}

func (v *MemoryVault) Get(key string) (Credentials, error) {
	if v == nil {
		return Credentials{}, ErrVaultMiss
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	item, ok := v.items[key]
	if !ok {
		return Credentials{}, ErrVaultMiss
	}
	return item, nil
}

func (v *MemoryVault) Put(key string, creds Credentials) error {
	if v == nil {
		return errors.New("vault is unavailable")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.items == nil {
		v.items = make(map[string]Credentials)
	}
	v.items[key] = creds
	return nil
}

func (v *MemoryVault) Delete(key string) error {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.items, key)
	return nil
}

type osVault struct{}

// OSVault stores secrets in macOS Keychain, Windows Credential Manager, or the
// Linux secret service. Failures are returned to the caller; they must never
// block a live sign-in.
func OSVault() Vault {
	return osVault{}
}

func (osVault) Get(key string) (Credentials, error) {
	secret, err := keyring.Get(vaultService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credentials{}, ErrVaultMiss
	}
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func (osVault) Put(key string, creds Credentials) error {
	payload, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return keyring.Set(vaultService, key, string(payload))
}

func (osVault) Delete(key string) error {
	err := keyring.Delete(vaultService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
