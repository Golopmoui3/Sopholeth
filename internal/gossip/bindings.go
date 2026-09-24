package gossip

// PeerBinding fixes an ID's canonical HTTP(S) origin and enclave. Only roots
// have verified bindings; ordinary peers remain permissionless referrals.
type PeerBinding struct {
	Origin  string
	Enclave string
}

// SetRootBindings applies a verified manifest's canonical bindings. This is the
// only route-update path for established roots. It does not add peers or grant
// a root role. Discovery expiry must not clear these bindings: an outage cannot
// turn a known root ID into an unsigned first claim. A later verified manifest
// may change or remove a binding; removed roots retain their ordinary peer entry.
func (p *Protocol) SetRootBindings(bindings map[NodeID]PeerBinding) {
	p.peersMutex.Lock()
	defer p.peersMutex.Unlock()
	p.rootBindings = make(map[NodeID]PeerBinding, len(bindings))
	for id, binding := range bindings {
		p.rootBindings[id] = binding
		if peer := p.peers[id]; peer != nil {
			// Replace rather than mutate: sends already holding a peer snapshot
			// must not race a verified refresh.
			updated := *peer
			oldOrigin, _ := peer.Origin()
			updated.HTTPOrigin, updated.Enclave = binding.Origin, binding.Enclave
			p.peers[id] = &updated
			if oldOrigin != binding.Origin {
				delete(p.peerFailures, id)
			}
		}
	}
}
