package protonnode

import (
	"encoding/json"
	"time"
)

// statusJSON is the Status() contract from PROMPT.md 5.1. Fields are never
// removed once shipped; new ones are added with omitempty.
type statusJSON struct {
	State       string `json:"state"`
	Server      string `json:"server,omitempty"`
	EntryIP     string `json:"entryIP,omitempty"`
	SNI         string `json:"sni,omitempty"`
	HandshakeMS int64  `json:"handshakeMs,omitempty"`
	UptimeSec   int64  `json:"uptimeSec,omitempty"`
	RxBytes     int64  `json:"rxBytes"`
	TxBytes     int64  `json:"txBytes"`
	LastError   string `json:"lastError,omitempty"`
	PID         int    `json:"pid"`
	UDP         bool   `json:"udp"`
	SocksPort   int    `json:"socksPort,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Reconnects  int    `json:"reconnects"`
	Plan        string `json:"plan,omitempty"`
	Tier        int    `json:"tier,omitempty"`
}

// Status is an atomic, non-blocking snapshot: it never touches the network and
// only reads counters from the local WireGuard UAPI.
func Status() string {
	eng.mu.Lock()
	st := statusJSON{
		State:     eng.state,
		LastError: eng.lastError,
		PID:       pid(),
		UDP:       false, // P1: SOCKS5 is TCP-only; P2 flips this to true
		Plan:      eng.plan,
		Tier:      eng.tier,
		Mode:      eng.runCfg.Mode,
	}
	run := eng.run
	eng.mu.Unlock()

	if run != nil {
		run.mu.Lock()
		st.Server = run.server
		st.EntryIP = run.entryIP
		st.SNI = run.sni
		st.HandshakeMS = run.handshakeMS
		st.Reconnects = run.reconnects
		st.SocksPort = run.cfg.SocksPort
		if st.LastError == "" {
			st.LastError = run.lastError
		}
		if !run.startedAt.IsZero() && eng.stateSnapshot() == stateConnected {
			st.UptimeSec = int64(time.Since(run.startedAt).Seconds())
		}
		tun := run.tun
		run.mu.Unlock()
		if tun != nil {
			if rx, tx, _, err := tun.Stats(); err == nil {
				st.RxBytes, st.TxBytes = rx, tx
			}
		}
	}
	buf, _ := json.Marshal(st)
	return string(buf)
}
