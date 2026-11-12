// Package art draws the Wake Desk pictures: the night room from the website
// with the machine in it, a small icon per machine type, and the power logo.
// Everything is drawn with the standard library at any pixel size, so the
// same picture can go to a graphics-capable terminal or be reduced to
// half-block cells.
package art

import "strings"

// Kind is the type of machine a picture shows. It is stored in the
// device_type column; any other stored value means "guess from the name".
type Kind string

const (
	Desktop Kind = "desktop"
	Laptop  Kind = "laptop"
	Server  Kind = "server"
	NAS     Kind = "nas"
	Mini    Kind = "mini"
	Console Kind = "console"
)

// Kinds lists every machine type in the order the form cycles through them.
func Kinds() []Kind { return []Kind{Desktop, Laptop, Server, NAS, Mini, Console} }

// Label is the short human name of a machine type.
func (k Kind) Label() string {
	switch k {
	case NAS:
		return "NAS"
	case Mini:
		return "mini PC"
	default:
		return string(k)
	}
}

// ParseKind accepts a stored or typed machine type.
func ParseKind(value string) (Kind, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "mini pc", "mini-pc", "minipc":
		return Mini, true
	}
	for _, k := range Kinds() {
		if value == string(k) {
			return k, true
		}
	}
	return "", false
}

// Resolve returns the stored type when it is known, otherwise a guess from
// the machine name. The boolean reports whether the type was chosen by hand.
func Resolve(stored, name string) (Kind, bool) {
	if k, ok := ParseKind(stored); ok {
		return k, true
	}
	return Guess(name), false
}

// Guess picks a machine type from words commonly used in host names. Short
// words must match a whole name part ("pi" matches "lab-pi" but not
// "pixel"); longer words may appear anywhere.
func Guess(name string) Kind {
	n := strings.ToLower(name)
	parts := strings.FieldsFunc(n, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	has := func(words ...string) bool {
		for _, w := range words {
			if len(w) > 3 && strings.Contains(n, w) {
				return true
			}
			for _, p := range parts {
				if p == w {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("nas", "synology", "qnap", "truenas", "unraid", "vault", "storage", "backup"):
		return NAS
	case has("laptop", "macbook", "thinkpad", "notebook", "xps"):
		return Laptop
	case has("server", "srv", "rack", "proxmox", "pve", "esxi", "node", "hypervisor"):
		return Server
	case has("mini", "nuc", "pi", "rpi", "raspberry", "micro", "tiny"):
		return Mini
	case has("console", "xbox", "playstation", "ps5", "ps4", "tv", "steamdeck", "htpc", "media"):
		return Console
	default:
		return Desktop
	}
}
