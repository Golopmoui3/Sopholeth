package trust

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

// This signature uses the identity point for R and zero for S. No private
// key is involved. The old all-zero anchor accepts some of these messages
// under Go 1.22; all must be rejected regardless of the stdlib's behavior.
func TestRejectsForgedPlaceholder(t *testing.T) {
	placeholder := make(ed25519.PublicKey, ed25519.PublicKeySize)
	signature := make([]byte, ed25519.SignatureSize)
	signature[0] = 1
	now := time.Unix(1_900_000_000, 0)
	for i := int64(0); i < 256; i++ {
		list := &SignedList{
			Version: OmegaVersion, Expires: 2_000_000_000 + i,
			Nodes: []string{"untrusted.invalid:8080"}, Signature: signature,
		}
		if err := list.Verify(placeholder, now); !errors.Is(err, ErrUnconfiguredAnchor()) {
			t.Fatalf("variant %d: want unconfigured anchor, got %v", i, err)
		}
		resolver := &stubResolver{records: map[string][]string{
			DefaultBootstrapName:  {"omega=" + DefaultSignedListName},
			DefaultSignedListName: {list.Encode()},
		}}
		if got, err := FetchSigned(context.Background(), DNSConfig{Resolver: resolver}, placeholder, now); err == nil || got != nil {
			t.Fatalf("accepted forged DNS metadata (variant %d): %v, %v", i, got, err)
		}
		dir := t.TempDir()
		if err := SaveCache(dir, list); err != nil {
			t.Fatal(err)
		}
		cached, err := LoadCache(dir)
		if err != nil || cached == nil {
			t.Fatalf("load forged cache: %v, %v", cached, err)
		}
		if err := cached.Verify(placeholder, now); !errors.Is(err, ErrUnconfiguredAnchor()) {
			t.Fatalf("cached variant %d: want unconfigured anchor, got %v", i, err)
		}
	}
}

func TestDecodeOmegaPubkey(t *testing.T) {
	for _, encoded := range []string{"", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="} {
		if key, err := decodeOmegaPubkey(encoded); key != nil || !errors.Is(err, ErrUnconfiguredAnchor()) {
			t.Errorf("%q: want unconfigured anchor, got %x, %v", encoded, key, err)
		}
	}
	for _, encoded := range []string{"not base64", "AA==", base64.StdEncoding.EncodeToString(make([]byte, 33))} {
		if key, err := decodeOmegaPubkey(encoded); key != nil || err == nil {
			t.Errorf("%q: accepted malformed anchor", encoded)
		}
	}
	pub, priv := testKeypair(t)
	key, err := decodeOmegaPubkey(base64.StdEncoding.EncodeToString(pub))
	if err != nil || !key.Equal(pub) {
		t.Fatalf("generated anchor: %x, %v", key, err)
	}
	list := validList(time.Now().Add(time.Hour).Unix())
	list.Sign(priv)
	if err := list.Verify(key, time.Now()); err != nil {
		t.Fatalf("generated anchor must remain usable: %v", err)
	}
}

type forbiddenResolver struct{ t *testing.T }

func (r forbiddenResolver) LookupTXT(context.Context, string) ([]string, error) {
	r.t.Fatal("DNS must not be consulted with an unconfigured or malformed anchor")
	return nil, nil
}

func TestFetchRejectsAnchorBeforeDNS(t *testing.T) {
	for _, key := range []ed25519.PublicKey{nil, {}, make([]byte, 32), {1, 2}} {
		list, err := FetchSigned(context.Background(), DNSConfig{Resolver: forbiddenResolver{t}}, key, time.Now())
		if list != nil || err == nil {
			t.Fatalf("invalid anchor %x accepted: %v, %v", key, list, err)
		}
	}
}
