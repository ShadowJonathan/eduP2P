package toversok

import (
	"context"
	"errors"
	"fmt"
	"github.com/shadowjonathan/edup2p/types"
	"github.com/shadowjonathan/edup2p/types/ifaces"
	"github.com/shadowjonathan/edup2p/types/key"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

//type EngineOptions struct {
//	//Ctx context.Context
//	//Ccc context.CancelCauseFunc
//	//
//	//PrivKey key.NakedKey
//	//
//	//Control    dial.Opts
//	//ControlKey key.ControlPublic
//	//
//	//// Do not contact control
//	//OverrideControl bool
//	//OverrideIPv4    netip.Prefix
//	//OverrideIPv6    netip.Prefix
//
//	WG WireGuardHost
//	FW FirewallHost
//	Co ControlHost
//
//	ExtBindPort uint16
//
//	PrivateKey key.NodePrivate
//}

type Engine struct {
	ctx context.Context
	ccc context.CancelCauseFunc

	sess *Session

	extBind *types.UDPConnCloseCatcher
	extPort uint16

	wg WireGuardHost
	fw FirewallHost
	co ControlHost

	nodePriv key.NodePrivate

	started bool
}

func (e *Engine) Start() error {
	if e.started {
		return nil
	}

	if err := e.InstallSession(); err != nil {
		return fmt.Errorf("could not install session: %w", err)
	}

	e.started = true

	return nil
}

// StalledEngineRestartInterval represents how many seconds to wait before retrying creating a session,
// after creation fails.
const StalledEngineRestartInterval = time.Second * 10

func (e *Engine) Restart() {
	if e.ctx.Err() != nil {
		// If the engine has been cancelled, do nothing
		return
	}

	if e.sess.ctx.Err() == nil {
		// Session is still running
		e.sess.ccc(errors.New("restarting"))
	}

	var err error
	defer func() {
		if err != nil {
			slog.Info("restart: will retry in 10 seconds")
			time.AfterFunc(StalledEngineRestartInterval, e.Restart)
		}
	}()

	if err = e.wg.Reset(); err != nil {
		e.slog().Error("restart: could not reset wireguard", "err", err)
		return
	}

	if err = e.fw.Reset(); err != nil {
		e.slog().Error("restart: could not reset firewall", "err", err)
		return
	}

	if err = e.InstallSession(); err != nil {
		e.slog().Error("restart: could not install session", "err", err)
		return
	}
}

func (e *Engine) Stop() {
	if e.sess.ctx.Err() != nil {
		e.sess.ccc(errors.New("shutting down"))
	}

	e.wg.Reset()
	e.fw.Reset()
}

func (e *Engine) InstallSession() error {
	var err error
	e.sess, err = SetupSession(e.ctx, e.wg, e.fw, e.co, e.getExtConn, e.getNodePriv)
	if err != nil {
		return fmt.Errorf("failed to setup session: %w", err)
	}

	context.AfterFunc(e.sess.ctx, e.Restart)

	e.sess.Start()

	return err
}

func (e *Engine) Started() bool {
	return e.started
}

func (e *Engine) slog() *slog.Logger {
	return slog.With("from", "engine")
}

// TODO add status update event channels (to display connection status, control status, session status, IP, etc.)

// NewEngine creates a new engine and initiates it
func NewEngine(
	parentCtx context.Context,
	wg WireGuardHost,
	fw FirewallHost,
	co ControlHost,

	extBindPort uint16,

	privateKey key.NodePrivate,
) (*Engine, error) {
	ctx, ccc := context.WithCancelCause(parentCtx)

	if wg == nil {
		return nil, errors.New("cannot initialise toversok engine with nil WireGuardHost")
	} else if fw == nil {
		return nil, errors.New("cannot initialise toversok engine with nil FirewallHost")
	} else if co == nil {
		return nil, errors.New("cannot initialise toversok engine with nil ControlHost")
	} else if privateKey.IsZero() {
		return nil, errors.New("cannot initialise toversok engine with zero privateKey")
	}

	return &Engine{
		ctx:  ctx,
		ccc:  ccc,
		sess: nil,

		extBind: nil,
		extPort: extBindPort,

		wg: wg,
		fw: fw,
		co: co,

		nodePriv: privateKey,
		started:  false,
	}, nil
}

func (e *Engine) getNodePriv() *key.NodePrivate {
	return &e.nodePriv
}

func (e *Engine) getExtConn() types.UDPConn {
	if e.extBind == nil || e.extBind.Closed {
		conn, err := e.bindExt()

		if err != nil {
			panic(fmt.Sprintf("could not bind ext: %s", err))
		}

		slog.Info("created ext sock", "addr", conn.LocalAddr().String(), "extPort", e.extPort)

		e.extBind = &types.UDPConnCloseCatcher{
			UDPConn: conn,
			Closed:  false,
		}
	}

	return e.extBind
}

func (e *Engine) bindExt() (*net.UDPConn, error) {
	ua := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.IPv4Unspecified(), e.extPort)) // 42069

	return net.ListenUDP("udp", ua)
}

//
//const WGKeepAlive = time.Second * 20
//
//func (e *Engine) Handle(ev Event) error {
//	switch ev := ev.(type) {
//	case PeerAddition:
//		return e.AddPeer(ev.Key, ev.HomeRelayId, ev.Endpoints, ev.SessionKey, ev.VIPs.IPv4, ev.VIPs.IPv6)
//	case PeerUpdate:
//		// FIXME the reason for the panic below is because this function is essentially deprecated, and it still uses
//		//  gonull, which is a pain
//		panic("cannot handle PeerUpdate via handle")
//
//		//if ev.Endpoints.Present {
//		//	if err := e.stage.SetEndpoints(ev.Key, ev.Endpoints.Val); err != nil {
//		//		return fmt.Errorf("failed to update endpoints: %w", err)
//		//	}
//		//}
//		//
//		//if ev.SessionKey.Present {
//		//	if err := e.stage.UpdateSessionKey(ev.Key, ev.SessionKey.Val); err != nil {
//		//		return fmt.Errorf("failed to update session key: %w", err)
//		//	}
//		//}
//		//
//		//if ev.HomeRelayId.Present {
//		//	if err := e.stage.UpdateHomeRelay(ev.Key, ev.HomeRelayId.Val); err != nil {
//		//		return fmt.Errorf("failed to update home relay: %w", err)
//		//	}
//		//}
//	case PeerRemoval:
//		return e.RemovePeer(ev.Key)
//	case RelayUpdate:
//		return e.UpdateRelays(ev.Set)
//	default:
//		// TODO warn-log about unknown type instead of panic
//		panic("Unknown type!")
//	}
//
//	return nil
//}
//
//func (e *Engine) AddPeer(peer key.NodePublic, homeRelay int64, endpoints []netip.AddrPort, session key.SessionPublic, ip4 netip.Addr, ip6 netip.Addr) error {
//	m := e.bindLocal()
//	e.localMapping[peer] = m
//
//	if err := e.wg.UpdatePeer(peer, PeerCfg{
//		Set: true,
//		VIPs: &VirtualIPs{
//			IPv4: ip4,
//			IPv6: ip6,
//		},
//		KeepAliveInterval: nil,
//		LocalEndpointPort: &m.port,
//	}); err != nil {
//		return fmt.Errorf("failed to update wireguard: %w", err)
//	}
//
//	if err := e.stage.AddPeer(peer, homeRelay, endpoints, session, ip4, ip6); err != nil {
//		return fmt.Errorf("failed to update stage: %w", err)
//	}
//	return nil
//}
//
//func (e *Engine) UpdatePeer(peer key.NodePublic, homeRelay *int64, endpoints []netip.AddrPort, session *key.SessionPublic) error {
//	return e.stage.UpdatePeer(peer, homeRelay, endpoints, session)
//}
//
//func (e *Engine) RemovePeer(peer key.NodePublic) error {
//	if err := e.stage.RemovePeer(peer); err != nil {
//		return err
//	}
//
//	if err := e.wg.RemovePeer(peer); err != nil {
//		return fmt.Errorf("failed to remove peer from wireguard: %w", err)
//	}
//
//	return nil
//}
//
//func (e *Engine) UpdateRelays(relay []relay.Information) error {
//	return e.stage.UpdateRelays(relay)
//}

type FakeControl struct {
	controlKey key.ControlPublic
	ipv4       netip.Prefix
	ipv6       netip.Prefix
}

func (f *FakeControl) ControlKey() key.ControlPublic {
	return f.controlKey
}

func (f *FakeControl) IPv4() netip.Prefix {
	return f.ipv4
}

func (f *FakeControl) IPv6() netip.Prefix {
	return f.ipv6
}

func (f *FakeControl) InstallCallbacks(callbacks ifaces.ControlCallbacks) error {
	// NOP
	return nil
}

func (f *FakeControl) UpdateEndpoints(ports []netip.AddrPort) error {
	// NOP
	return nil
}
