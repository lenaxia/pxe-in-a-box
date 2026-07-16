package config

import (
	"fmt"
	"net"
	"regexp"

	"gopkg.in/yaml.v3"
)

// MAC is a normalized MAC address in colon-separated lowercase form
// (e.g., "00:e0:4c:68:00:8e"). This matches matchbox's internal
// normalization via Go's net.ParseMAC.
type MAC string

// NewMAC parses and normalizes a MAC address string.
// Accepts common formats: "00:e0:4c:68:00:8e", "00-E0-4C-68-00-8E",
// "00e04c68008e", etc. Output is always colon-separated lowercase.
func NewMAC(s string) (MAC, error) {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return "", fmt.Errorf("invalid MAC address %q: %w", s, err)
	}
	return MAC(hw.String()), nil
}

// String returns the MAC address as a normalized string.
func (m MAC) String() string {
	return string(m)
}

// UnmarshalYAML implements yaml.Unmarshaler for MAC addresses.
// Parsing fails early on invalid MACs during config loading.
func (m *MAC) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	mac, err := NewMAC(s)
	if err != nil {
		return err
	}
	*m = mac
	return nil
}

// hostnamePattern ensures hostnames are DNS-safe.
var hostnamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// isValidHostname returns true if the hostname is DNS-safe (RFC 1123 compliant,
// single label — no dots).
func isValidHostname(s string) bool {
	return hostnamePattern.MatchString(s)
}

// isValidAssetID returns true if the asset ID contains only lowercase
// alphanumeric characters, hyphens, and dots (for version numbers like
// talos-v1.10.6). Must start and end with alphanumeric.
func isValidAssetID(s string) bool {
	matched, _ := regexp.MatchString(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`, s)
	return matched
}
