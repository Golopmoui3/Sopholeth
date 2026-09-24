package gossip

import "testing"

func TestUnsignedAdvertisementsCannotMovePeer(t *testing.T) {
	for _, kind := range []string{"ordinary", "root"} {
		t.Run(kind, func(t *testing.T) {
			p, _ := newTestProtocol()
			peer := Node{ID: "peer", HTTPOrigin: "https://peer.example", Enclave: "default"}
			if kind == "root" {
				p.SetRootBindings(map[NodeID]PeerBinding{peer.ID: {Origin: peer.HTTPOrigin, Enclave: peer.Enclave}})
			}
			if !p.addPeer(&peer) {
				t.Fatal("valid peer could not join")
			}
			for _, change := range []Node{
				{ID: peer.ID, HTTPOrigin: "http://attacker.example:9", Enclave: peer.Enclave},
				{ID: peer.ID, HTTPOrigin: peer.HTTPOrigin, Enclave: "other"},
				{ID: peer.ID, Address: "peer.example", HTTPPort: 443, Enclave: peer.Enclave}, // HTTPS downgrade via legacy fields.
			} {
				if p.HandleBootstrap(&BootstrapRequest{NodeID: string(change.ID), HTTPOrigin: change.HTTPOrigin, Address: change.Address, HTTPPort: change.HTTPPort, Enclave: change.Enclave}).Success {
					t.Fatalf("bootstrap accepted conflicting route: %+v", change)
				}
				for _, typ := range []MessageType{MessageTypeSync, MessageTypeSyncRequest} {
					if err := p.handleSync(&Message{Type: typ, From: peer.ID, NodeInfo: &change}); err == nil {
						t.Fatalf("%s accepted conflicting route: %+v", typ, change)
					}
				}
				if err := p.handlePong(&Message{From: "someone-else", NodeInfo: &change}); err == nil {
					t.Fatal("PONG accepted mismatched sender/node ID")
				}
				// Even naming the same sender cannot change the enclave via PONG.
				if err := p.handlePong(&Message{From: peer.ID, NodeInfo: &change}); err != nil {
					t.Fatal(err)
				}
				if got := p.GetPeers(); len(got) != 1 || *got[0] != peer {
					t.Fatalf("unsigned advertisement changed peer: %+v", got)
				}
			}
			// Equivalent URL spelling and a missing default enclave are not moves.
			if !p.addPeer(&Node{ID: peer.ID, HTTPOrigin: "https://PEER.example:443/"}) {
				t.Fatal("rejected unchanged canonical route")
			}
			if kind == "root" {
				p.removePeer(peer.ID)
				if p.addPeer(&Node{ID: peer.ID, HTTPOrigin: "http://attacker.example:9"}) {
					t.Fatal("eviction released the signed root binding")
				}
				if !p.addPeer(&peer) {
					t.Fatal("evicted root could not rejoin at signed origin")
				}
			}
		})
	}
}
