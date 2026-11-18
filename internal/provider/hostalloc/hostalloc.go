// Package hostalloc records what combinations share on a host, in host.json
// under an flock, so concurrent wendy processes never hand out the same
// port or subnet and the last user of a shared service removes it.
package hostalloc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
)

// Host is host.json.
type Host struct {
	// BMCPorts maps "<env id>/<machine>" to a port in the shared vBMC.
	BMCPorts map[string]int `json:"bmcPorts"`
	// Subnets6 maps an environment id to its index of fd00:ca7e::/48.
	Subnets6 map[string]int `json:"subnets6"`
	// Users of each shared service, by environment id.
	Users map[string][]string `json:"users"`
	// PoolCreated says wendy, not the user, created libvirt's default pool.
	PoolCreated bool `json:"poolCreated,omitempty"`
}

// Update reads host.json under an exclusive lock, applies fn, and writes it
// back if fn succeeded.
func Update(data string, fn func(*Host) error) error {
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(data, "host.json.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking host.json: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck // Closing the file releases the lock anyway.

	path := filepath.Join(data, "host.json")
	h := &Host{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, h); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if h.BMCPorts == nil {
		h.BMCPorts = map[string]int{}
	}
	if h.Subnets6 == nil {
		h.Subnets6 = map[string]int{}
	}
	if h.Users == nil {
		h.Users = map[string][]string{}
	}
	if err := fn(h); err != nil {
		return err
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// BMCPortBase is the first port handed out in the shared vBMC.
const BMCPortBase = 16230

// BMCPort returns the port of a machine, allocating the lowest free one.
func (h *Host) BMCPort(envID, machine string) int {
	key := envID + "/" + machine
	if p, ok := h.BMCPorts[key]; ok {
		return p
	}
	used := map[int]bool{}
	for _, p := range h.BMCPorts {
		used[p] = true
	}
	p := BMCPortBase
	for used[p] {
		p++
	}
	h.BMCPorts[key] = p
	return p
}

// ReleaseBMCPorts frees every port of an environment.
func (h *Host) ReleaseBMCPorts(envID string) {
	for k := range h.BMCPorts {
		if len(k) > len(envID) && k[:len(envID)+1] == envID+"/" {
			delete(h.BMCPorts, k)
		}
	}
}

// Subnet6 returns an environment's IPv6 /64, allocating the lowest free one.
// skip reports subnets something else on the host already uses.
func (h *Host) Subnet6(envID string, skip func(string) bool) (string, error) {
	if i, ok := h.Subnets6[envID]; ok {
		return subnet6(i), nil
	}
	used := map[int]bool{}
	for _, i := range h.Subnets6 {
		used[i] = true
	}
	for i := 1; i < 0xffff; i++ {
		if used[i] || (skip != nil && skip(subnet6(i))) {
			continue
		}
		h.Subnets6[envID] = i
		return subnet6(i), nil
	}
	return "", errors.New("no free IPv6 /64")
}

func subnet6(i int) string { return fmt.Sprintf("fd00:ca7e:%x::/64", i) }

// AddUser records that an environment uses a shared service.
func (h *Host) AddUser(service, envID string) {
	if !slices.Contains(h.Users[service], envID) {
		h.Users[service] = append(h.Users[service], envID)
	}
}

// RemoveUser records that an environment is done with a shared service, and
// reports whether anyone still uses it.
func (h *Host) RemoveUser(service, envID string) (stillUsed bool) {
	h.Users[service] = slices.DeleteFunc(h.Users[service], func(s string) bool { return s == envID })
	if len(h.Users[service]) == 0 {
		delete(h.Users, service)
		return false
	}
	return true
}
