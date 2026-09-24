package discovery

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"sopholeth/internal/testomega"
	"sopholeth/internal/trust/bootstrap"
)

func checkFingerprint(b bootstrap.Bundle, expected string) error {
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("OMEGA_EXPECTED_SHA256 must be an independently verified 64-hex-digit initial TUF root fingerprint")
	}
	if err := b.Validate(); err != nil {
		return err
	}
	if b.Fingerprint() != hex.EncodeToString(decoded) {
		return fmt.Errorf("bundled omega fingerprint %s does not match expected %s", b.Fingerprint(), expected)
	}
	return nil
}

func TestPublicReleaseBundle(t *testing.T) {
	expected, enabled := os.LookupEnv("OMEGA_EXPECTED_SHA256")
	if !enabled {
		t.Skip("run make check-public-release with OMEGA_EXPECTED_SHA256")
	}
	b, err := PublicBundle()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFingerprint(b, expected); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseFingerprint(t *testing.T) {
	r := testomega.New(t, []bootstrap.Root{{ID: "one", Origin: "https://one.invalid"}, {ID: "two", Origin: "https://two.invalid"}, {ID: "three", Origin: "https://three.invalid"}})
	equivalent := r.Bundle
	equivalent.Repository += "/"
	if BundleIdentity(equivalent) != BundleIdentity(r.Bundle) {
		t.Fatal("equivalent repository spelling changed the saved identity")
	}
	if err := checkFingerprint(r.Bundle, r.Bundle.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "not hex", "00", strings.Repeat("0", 64)} {
		if err := checkFingerprint(r.Bundle, value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
