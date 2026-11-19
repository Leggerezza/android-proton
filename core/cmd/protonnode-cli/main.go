// Command protonnode-cli drives core/protonnode for verification on a desktop
// and, cross-compiled for android/arm64, on a device through adb shell.
//
// It is NOT part of the AAR (gomobile binds ./protonnode only). The CLI exists
// so P1 can be judged by a command whose output is real network evidence
// (PROMPT.md 11: "in-tunnel TCP request succeeds"), not by a screenshot.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dsh/android-proton/core/protonnode"
)

const version = "0.1.0"

func usage() {
	fmt.Fprintf(os.Stderr, "protonnode-cli "+version+" - P1 harness for core/protonnode\n\n"+
		"Usage:\n  protonnode-cli <command> [flags]\n\n"+
		"Commands:\n"+
		"  selftest   connect, fetch a URL through the SOCKS5 port, print the exit IP\n"+
		"  run        connect and keep the SOCKS5 port open until interrupted\n"+
		"  udptest    connect, then send a DNS query over SOCKS5 UDP ASSOCIATE (P2)\n"+
		"  state      print SessionState() and Status()\n"+
		"  convert    turn a Windows-baseline state.json into the session JSON this tool takes\n\n"+
		"Flags are per command; run \"protonnode-cli <command> -h\".\n")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch strings.TrimLeft(os.Args[1], "-") {
	case "selftest":
		err = cmdSelftest(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "state":
		err = cmdState(os.Args[2:])
	case "udptest":
		err = cmdUDPTest(os.Args[2:])
	case "convert":
		err = cmdConvert(os.Args[2:])
	case "version":
		fmt.Println("protonnode-cli " + version)
	case "help", "h":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// commonFlags are shared by selftest and run.
type commonFlags struct {
	state      string
	cfgDir     string
	socks      int
	server     string
	country    string
	prefer     string
	mode       string
	secureCore bool
	lock       bool
	noLock     bool
	mtu        int
	sni        string
	fp         string
	handshake  int
	dns        string
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.state, "state", "", "session JSON produced by the convert command; falls back to PROTONNODE_SESSION")
	fs.StringVar(&c.cfgDir, "cfg", "", "cache directory (default: <tmp>/protonnode)")
	fs.IntVar(&c.socks, "socks", 1080, "loopback SOCKS5 port")
	fs.StringVar(&c.server, "server", "", "logical server name, e.g. JP#377")
	fs.StringVar(&c.country, "country", "", "exit country code, e.g. JP")
	fs.StringVar(&c.prefer, "prefer", "", "comma separated country preference order")
	fs.StringVar(&c.mode, "mode", "normal", "normal|game")
	fs.BoolVar(&c.secureCore, "secure-core", false, "require Secure Core")
	fs.BoolVar(&c.lock, "lock-server", false, "keep the current server across reconnects")
	fs.BoolVar(&c.noLock, "no-lock-server", false, "allow switching servers on reconnect")
	fs.IntVar(&c.mtu, "mtu", 0, "tunnel MTU (default 1320)")
	fs.StringVar(&c.sni, "sni", "", "override the TLS SNI of the entry")
	fs.StringVar(&c.fp, "fingerprint", "chrome", "utls fingerprint: chrome|firefox|safari|edge|golang")
	fs.IntVar(&c.handshake, "handshake-timeout", 0, "WireGuard handshake timeout in seconds")
	fs.StringVar(&c.dns, "dns", "", "pin the DNS server used for API lookups, e.g. 172.26.26.3")
}

func (c *commonFlags) configJSON() string {
	cfg := map[string]any{
		"mode":                c.mode,
		"server":              c.server,
		"country":             c.country,
		"secureCore":          c.secureCore,
		"socksPort":           c.socks,
		"mtu":                 c.mtu,
		"sni":                 c.sni,
		"fingerprint":         c.fp,
		"handshakeTimeoutSec": c.handshake,
	}
	if c.prefer != "" {
		parts := strings.Split(c.prefer, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		cfg["prefer"] = parts
	}
	if c.lock {
		cfg["lockServer"] = true
	}
	if c.noLock {
		cfg["lockServer"] = false
	}
	buf, _ := json.Marshal(cfg)
	return string(buf)
}

func (c *commonFlags) bootstrap() error {
	dir := c.cfgDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "protonnode")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := protonnode.Init(dir); err != nil {
		return err
	}
	if c.dns != "" {
		if err := protonnode.SetAPIDNS(c.dns); err != nil {
			return err
		}
	}
	path := c.state
	if path == "" {
		path = os.Getenv("PROTONNODE_SESSION")
	}
	if path == "" {
		return fmt.Errorf("-state is required (or set PROTONNODE_SESSION)")
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := protonnode.SetSession(string(buf)); err != nil {
		return err
	}
	fmt.Println("session:", protonnode.SessionState())
	return nil
}

func cmdSelftest(args []string) error {
	fs := flag.NewFlagSet("selftest", flag.ExitOnError)
	var c commonFlags
	c.register(fs)
	url := fs.String("url", "http://api.ipify.org/", "URL fetched through the tunnel")
	wait := fs.Duration("wait", 90*time.Second, "how long to wait for the connected state")
	_ = fs.Parse(args)
	if err := c.bootstrap(); err != nil {
		return err
	}
	defer protonnode.Shutdown()

	started := time.Now()
	if _, err := protonnode.Start(c.configJSON()); err != nil {
		return err
	}
	if err := waitConnected(*wait); err != nil {
		fmt.Println("status:", protonnode.Status())
		fmt.Println("--- logs ---")
		fmt.Println(protonnode.RecentLogs(40))
		_ = protonnode.Stop()
		return err
	}
	fmt.Printf("connected after %s\n", time.Since(started).Round(time.Millisecond))
	fmt.Println("status:", protonnode.Status())

	saveSessionBack(c.state)

	body, err := socksGet(fmt.Sprintf("127.0.0.1:%d", c.socks), *url, 25*time.Second)
	if err != nil {
		fmt.Println("--- logs ---")
		fmt.Println(protonnode.RecentLogs(40))
		_ = protonnode.Stop()
		return fmt.Errorf("in-tunnel request failed: %w", err)
	}
	fmt.Printf("in-tunnel GET %s -> %s\n", *url, strings.TrimSpace(body))
	fmt.Println("status:", protonnode.Status())
	fmt.Println("--- logs ---")
	fmt.Println(protonnode.RecentLogs(20))
	if err := protonnode.Stop(); err != nil {
		return err
	}
	fmt.Println("stopped:", protonnode.Status())
	return nil
}

func waitConnected(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var st struct {
			State     string `json:"state"`
			LastError string `json:"lastError"`
		}
		_ = json.Unmarshal([]byte(protonnode.Status()), &st)
		if st.State == "connected" {
			return nil
		}
		if st.LastError != "" {
			fmt.Fprintf(os.Stderr, "waiting: %s (%s)\n", st.State, st.LastError)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("tunnel did not reach the connected state within %s", timeout)
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var c commonFlags
	c.register(fs)
	_ = fs.Parse(args)
	if err := c.bootstrap(); err != nil {
		return err
	}
	defer protonnode.Shutdown()
	if _, err := protonnode.Start(c.configJSON()); err != nil {
		return err
	}
	fmt.Printf("SOCKS5 on 127.0.0.1:%d (Ctrl-C to stop)\n", c.socks)
	for {
		time.Sleep(10 * time.Second)
		fmt.Println("status:", protonnode.Status())
	}
}

// saveSessionBack persists a rotated token pair. Proton refresh tokens are
// single-use: if the new pair is not written back, the next run fails with
// "401 Invalid access token" / "2001 Invalid input" even though the account is fine.
func saveSessionBack(path string) {
	if path == "" {
		return
	}
	sess, err := protonnode.SessionJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: cannot export the session:", err)
		return
	}
	if err := os.WriteFile(path, []byte(sess), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "warning: cannot persist the rotated session:", err)
		return
	}
	fmt.Println("session: rotated tokens written back to", path)
}

func cmdState(args []string) error {
	fs := flag.NewFlagSet("state", flag.ExitOnError)
	var c commonFlags
	c.register(fs)
	_ = fs.Parse(args)
	if err := c.bootstrap(); err != nil {
		return err
	}
	fmt.Println("status:", protonnode.Status())
	fmt.Println("session:", protonnode.SessionState())
	return nil
}

// cmdConvert maps the Windows baseline state.json to the session JSON this
// tool accepts. It prints no token material.
func cmdConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	from := fs.String("from", "", "path to the baseline proton-stealth-state.json")
	out := fs.String("out", "", "where to write the session JSON (0600)")
	_ = fs.Parse(args)
	if *from == "" || *out == "" {
		return fmt.Errorf("-from and -out are required")
	}
	raw, err := os.ReadFile(*from)
	if err != nil {
		return err
	}
	var in struct {
		UID         string          `json:"uid"`
		Access      string          `json:"access_token"`
		Refresh     string          `json:"refresh_token"`
		Email       string          `json:"login_email"`
		Seed        string          `json:"ed25519_seed"`
		DeviceName  string          `json:"device_name"`
		Certificate json.RawMessage `json:"certificate"`
		LastServer  string          `json:"last_server"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	sess := map[string]any{
		"uid":           in.UID,
		"access_token":  in.Access,
		"refresh_token": in.Refresh,
		"login_email":   in.Email,
		"ed25519_seed":  in.Seed,
		"device_name":   in.DeviceName,
		"last_server":   in.LastServer,
		"updated_at":    time.Now().Format(time.RFC3339),
	}
	if len(in.Certificate) > 0 && string(in.Certificate) != "null" {
		sess["certificate"] = in.Certificate
	}
	buf, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, buf, 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes); device %s; uid %s...\n", *out, len(buf), in.DeviceName, firstN(in.UID, 8))
	return nil
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// socksGet performs a SOCKS5 CONNECT (domain ATYP) and an HTTP/1.1 GET,
// returning the response body. This is the same path mihomo takes.
func socksGet(proxyAddr, rawURL string, timeout time.Duration) (string, error) {
	host, port, path, err := splitURL(rawURL)
	if err != nil {
		return "", err
	}
	c, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		return "", err
	}
	greet := make([]byte, 2)
	if _, err := io.ReadFull(c, greet); err != nil {
		return "", err
	}
	if greet[0] != 5 || greet[1] != 0 {
		return "", fmt.Errorf("socks greeting rejected: %v", greet)
	}
	req := []byte{5, 1, 0, 3, byte(len(host))}
	req = append(req, host...)
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := c.Write(req); err != nil {
		return "", err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		return "", err
	}
	if head[1] != 0 {
		return "", fmt.Errorf("socks connect failed with reply %#x", head[1])
	}
	switch head[3] {
	case 1:
		if _, err := io.ReadFull(c, make([]byte, 4+2)); err != nil {
			return "", err
		}
	case 4:
		if _, err := io.ReadFull(c, make([]byte, 16+2)); err != nil {
			return "", err
		}
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(c, make([]byte, int(l[0])+2)); err != nil {
			return "", err
		}
	}
	fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: protonnode-selftest\r\nConnection: close\r\n\r\n", path, host)
	body, rerr := io.ReadAll(io.LimitReader(bufio.NewReader(c), 1<<20))
	if rerr != nil && len(body) == 0 {
		return "", rerr
	}
	resp := string(body)
	if i := strings.Index(resp, "\r\n\r\n"); i >= 0 {
		resp = resp[i+4:]
	}
	return strings.TrimSpace(resp), nil
}

func splitURL(raw string) (host string, port int, path string, err error) {
	s := raw
	if strings.HasPrefix(s, "https://") {
		return "", 0, "", fmt.Errorf("selftest speaks plain HTTP through SOCKS5 only; use http:// (TLS over SOCKS is covered by the tunnel test)")
	}
	s = strings.TrimPrefix(s, "http://")
	path = "/"
	if i := strings.Index(s, "/"); i >= 0 {
		path = s[i:]
		s = s[:i]
	}
	port = 80
	if i := strings.LastIndex(s, ":"); i >= 0 {
		if p, cerr := strconv.Atoi(s[i+1:]); cerr == nil {
			port = p
			s = s[:i]
		}
	}
	if s == "" {
		return "", 0, "", fmt.Errorf("cannot parse URL %q", raw)
	}
	return s, port, path, nil
}

// cmdUDPTest proves the SOCKS5 UDP ASSOCIATE path through a real tunnel: it
// sends a DNS query to a resolver on the internet through the relay and
// requires a real answer. PROMPT.md 5.4 / 11.
func cmdUDPTest(args []string) error {
	fs := flag.NewFlagSet("udptest", flag.ExitOnError)
	var c commonFlags
	c.register(fs)
	target := fs.String("target", "8.8.8.8:53", "UDP destination, resolved inside the tunnel")
	query := fs.String("query", "api.ipify.org", "DNS name to resolve through the relay")
	wait := fs.Duration("wait", 90*time.Second, "how long to wait for the connected state")
	_ = fs.Parse(args)
	if err := c.bootstrap(); err != nil {
		return err
	}
	defer protonnode.Shutdown()
	if _, err := protonnode.Start(c.configJSON()); err != nil {
		return err
	}
	if err := waitConnected(*wait); err != nil {
		fmt.Println("status:", protonnode.Status())
		fmt.Println(protonnode.RecentLogs(30))
		_ = protonnode.Stop()
		return err
	}
	fmt.Println("status:", protonnode.Status())

	proxy := fmt.Sprintf("127.0.0.1:%d", c.socks)
	control, relay, err := socksUDPAssociate(proxy, 10*time.Second)
	if err != nil {
		_ = protonnode.Stop()
		return err
	}
	defer control.Close()
	saveSessionBack(c.state)
	fmt.Println("udp relay:", relay.String())

	conn, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		_ = protonnode.Stop()
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	id := uint16(0x4a21)
	req := dnsQuery(id, *query)
	host, portStr, _ := net.SplitHostPort(*target)
	port, _ := strconv.Atoi(portStr)
	ip := net.ParseIP(host).To4()
	if ip == nil {
		_ = protonnode.Stop()
		return fmt.Errorf("-target must be ip:port in P2 (domain lookup inside UDP is P3)")
	}
	pkt := []byte{0, 0, 0, 1}
	pkt = append(pkt, ip...)
	pkt = binary.BigEndian.AppendUint16(pkt, uint16(port))
	pkt = append(pkt, req...)

	// UDP is best effort and has no retransmission: a datagram sent while the
	// WireGuard data path is still warming up is simply lost. A DNS client
	// retries, and so does this check (3 attempts, 5s each).
	buf := make([]byte, 4096)
	var udpN int
	var udpErr error
	attempts := 0
	for attempts = 1; attempts <= 3; attempts++ {
		if _, werr := conn.Write(pkt); werr != nil {
			_ = protonnode.Stop()
			return werr
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		udpN, udpErr = conn.Read(buf)
		if udpErr == nil {
			break
		}
		fmt.Printf("udp attempt %d: %v (retrying, UDP has no retransmission)\n", attempts, udpErr)
	}
	if udpErr != nil {
		fmt.Println(protonnode.RecentLogs(30))
		_ = protonnode.Stop()
		return fmt.Errorf("no UDP reply through the tunnel after %d attempts: %w", attempts-1, udpErr)
	}
	n := udpN
	_, _, payload, err := splitSocksUDP(buf[:n])
	if err != nil {
		_ = protonnode.Stop()
		return err
	}
	answers, ips, err := dnsParse(*target, payload, id)
	if err != nil {
		_ = protonnode.Stop()
		return err
	}
	fmt.Printf("UDP echo round-trip through Proton: %d bytes -> %d bytes, %d answer(s): %v (attempts: %d)\n", len(req), len(payload), answers, ips, attempts)
	fmt.Println("status:", protonnode.Status())
	if err := protonnode.Stop(); err != nil {
		return err
	}
	fmt.Println("stopped:", protonnode.Status())
	return nil
}

// socksUDPAssociate performs the SOCKS5 TCP control handshake for UDP.
func socksUDPAssociate(proxyAddr string, timeout time.Duration) (net.Conn, *net.UDPAddr, error) {
	c, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, nil, err
	}
	_ = c.SetDeadline(time.Now().Add(timeout))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		c.Close()
		return nil, nil, err
	}
	greet := make([]byte, 2)
	if _, err := io.ReadFull(c, greet); err != nil || greet[0] != 5 || greet[1] != 0 {
		c.Close()
		return nil, nil, fmt.Errorf("socks greeting failed")
	}
	if _, err := c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		c.Close()
		return nil, nil, err
	}
	head := make([]byte, 10)
	if _, err := io.ReadFull(c, head); err != nil {
		c.Close()
		return nil, nil, err
	}
	if head[1] != 0 {
		c.Close()
		return nil, nil, fmt.Errorf("UDP ASSOCIATE rejected with %#x", head[1])
	}
	relay := &net.UDPAddr{IP: net.IP(head[4:8]), Port: int(binary.BigEndian.Uint16(head[8:10]))}
	_ = c.SetDeadline(time.Time{})
	return c, relay, nil
}

// splitSocksUDP decodes RSV|FRAG|ATYP|DST|PORT|DATA (IPv4/IPv6/domain).
func splitSocksUDP(b []byte) (frag byte, dst string, payload []byte, err error) {
	if len(b) < 4 {
		return 0, "", nil, fmt.Errorf("short UDP datagram")
	}
	frag = b[2]
	p := 4
	var host string
	switch b[3] {
	case 1:
		if len(b) < p+4 {
			return 0, "", nil, fmt.Errorf("short IPv4")
		}
		host = net.IP(b[p : p+4]).String()
		p += 4
	case 4:
		if len(b) < p+16 {
			return 0, "", nil, fmt.Errorf("short IPv6")
		}
		host = net.IP(b[p : p+16]).String()
		p += 16
	case 3:
		if len(b) < p+1 {
			return 0, "", nil, fmt.Errorf("short domain")
		}
		l := int(b[p])
		p++
		if len(b) < p+l {
			return 0, "", nil, fmt.Errorf("short domain")
		}
		host = string(b[p : p+l])
		p += l
	default:
		return 0, "", nil, fmt.Errorf("unsupported ATYP %#x", b[3])
	}
	if len(b) < p+2 {
		return 0, "", nil, fmt.Errorf("short port")
	}
	port := int(binary.BigEndian.Uint16(b[p : p+2]))
	p += 2
	return frag, net.JoinHostPort(host, strconv.Itoa(port)), b[p:], nil
}

// dnsQuery builds a minimal A-record query.
func dnsQuery(id uint16, name string) []byte {
	out := binary.BigEndian.AppendUint16(nil, id)
	out = append(out, 0x01, 0x00) // RD
	out = append(out, 0x00, 0x01)                         // QDCOUNT
	out = append(out, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00) // ANCOUNT, NSCOUNT, ARCOUNT
	for _, label := range splitLabels(name) {
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	out = append(out, 0x00)
	out = binary.BigEndian.AppendUint16(out, 1) // QTYPE A
	out = binary.BigEndian.AppendUint16(out, 1) // QCLASS IN
	return out
}

func splitLabels(name string) []string {
	var out []string
	cur := ""
	for _, r := range name {
		if r == '.' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// dnsParse returns the answer count and the A records of a DNS response.
func dnsParse(target string, msg []byte, id uint16) (int, []string, error) {
	if len(msg) < 12 {
		return 0, nil, fmt.Errorf("short DNS response from %s", target)
	}
	if got := binary.BigEndian.Uint16(msg[0:2]); got != id {
		return 0, nil, fmt.Errorf("DNS id mismatch: got %#x want %#x", got, id)
	}
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))
	p := 12
	for i := 0; i < qd; i++ {
		for p < len(msg) {
			l := int(msg[p])
			p++
			if l == 0 {
				break
			}
			p += l
		}
		p += 4 // QTYPE + QCLASS
	}
	var ips []string
	for i := 0; i < an && p+10 <= len(msg); i++ {
		if msg[p]&0xc0 == 0xc0 {
			p += 2
		} else {
			for p < len(msg) {
				l := int(msg[p])
				p++
				if l == 0 {
					break
				}
				p += l
			}
		}
		if p+10 > len(msg) {
			break
		}
		typ := binary.BigEndian.Uint16(msg[p : p+2])
		rdlen := int(binary.BigEndian.Uint16(msg[p+8 : p+10]))
		p += 10
		if p+rdlen > len(msg) {
			break
		}
		if typ == 1 && rdlen == 4 {
			ips = append(ips, net.IP(msg[p:p+4]).String())
		}
		p += rdlen
	}
	if an == 0 {
		return 0, nil, fmt.Errorf("DNS response from %s carries no answer (NXDOMAIN/blocked?)", target)
	}
	return an, ips, nil
}
