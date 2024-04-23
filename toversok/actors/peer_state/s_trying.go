package peer_state

import (
	"github.com/shadowjonathan/edup2p/toversok/msg"
	"github.com/shadowjonathan/edup2p/types/key"
	"net/netip"
	"time"
)

type Trying struct {
	*StateCommon

	tryAt    time.Time
	attempts int
}

func (t *Trying) Name() string {
	return "trying"
}

func (t *Trying) OnTick() PeerState {
	if time.Now().After(t.tryAt) {
		return LogTransition(t, &EstPreTransmit{
			EstablishingCommon: mkEstComm(t.StateCommon, t.attempts),
		})
	}

	return nil
}

func (t *Trying) OnDirect(ap netip.AddrPort, clear *msg.ClearMessage) PeerState {
	if s := cascadeDirect(t, ap, clear); s != nil {
		return s
	}

	LogDirectMessage(t, ap, clear)

	switch m := clear.Message.(type) {
	case *msg.Ping:
		if !t.pingDirectValid(ap, clear.Session, m) {
			return nil
		}

		// TODO(jo): We could start establishing here, possibly.
		t.replyWithPongDirect(ap, clear.Session, m)
		return nil
	case *msg.Pong:
		t.ackPongDirect(ap, clear.Session, m)
		return nil
	//case *msg.Rendezvous:
	default:
		L(t).Info("ignoring direct session message",
			"ap", ap,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}

func (t *Trying) OnRelay(relay int64, peer key.NodePublic, clear *msg.ClearMessage) PeerState {
	if s := cascadeRelay(t, relay, peer, clear); s != nil {
		return s
	}

	LogRelayMessage(t, relay, peer, clear)

	switch m := clear.Message.(type) {
	case *msg.Ping:
		t.replyWithPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg.Pong:
		t.ackPongRelay(relay, peer, clear.Session, m)
		return nil
	case *msg.Rendezvous:
		return LogTransition(t, &EstRendezGot{
			EstablishingCommon: mkEstComm(t.StateCommon, 0),
			m:                  m,
		})
	default:
		L(t).Info("ignoring direct session message",
			"relay", relay,
			"peer", peer,
			"session", clear.Session,
			"msg", m.Debug())
		return nil
	}
}