package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConfigPath = "/etc/config/happ-openwrt"
	defaultOutputPath = "/etc/sing-box/config.json"
	maxDownloadBytes  = 2 << 20
)

type Options struct {
	Enabled      bool
	Subscription string
	Output       string
	ProxyMode    string
	DNSMode      string
	UserAgent    string
	HWID         string
	InstallID    string
	Restart      bool
}

type Node struct {
	Type       string
	Tag        string
	Server     string
	Port       int
	UUID       string
	Password   string
	Method     string
	Flow       string
	SNI        string
	AllowInsec bool
	TLS        bool
	Reality    *Reality
	Transport  map[string]any
}

type Reality struct {
	PublicKey string
	ShortID   string
}

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("happ-openwrt: %v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("happ-openwrt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", defaultConfigPath, "UCI config path")
	subscription := fs.String("subscription", "", "subscription URL or share link")
	output := fs.String("output", defaultOutputPath, "sing-box config path")
	mode := fs.String("mode", "", "proxy mode: global, rule, direct")
	dnsMode := fs.String("dns-mode", "", "dns mode: remote, local, system")
	userAgent := fs.String("user-agent", "", "custom subscription User-Agent")
	hwid := fs.String("hwid", "", "optional user-configured HWID")
	installID := fs.String("install-id", "", "optional user-configured InstallID")
	restart := fs.Bool("restart", false, "restart sing-box after successful update")
	if err := fs.Parse(args); err != nil {
		return usage()
	}

	cmd := "update"
	if fs.NArg() > 0 {
		cmd = fs.Arg(0)
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		return usage()
	}

	opts := Options{
		Enabled:   true,
		Output:    *output,
		ProxyMode: strings.ToLower(firstNonEmpty(*mode, "global")),
		DNSMode:   strings.ToLower(firstNonEmpty(*dnsMode, "remote")),
		Restart:   *restart,
	}
	if fileOpts, err := readUCIConfig(*configPath); err == nil {
		opts = mergeOptions(fileOpts, opts)
	}
	if *subscription != "" {
		opts.Subscription = *subscription
	}
	if *output != defaultOutputPath {
		opts.Output = *output
	}
	if *mode != "" {
		opts.ProxyMode = strings.ToLower(*mode)
	}
	if *dnsMode != "" {
		opts.DNSMode = strings.ToLower(*dnsMode)
	}
	if *userAgent != "" {
		opts.UserAgent = *userAgent
	}
	if *hwid != "" {
		opts.HWID = *hwid
	}
	if *installID != "" {
		opts.InstallID = *installID
	}
	if *restart {
		opts.Restart = true
	}

	switch cmd {
	case "parse":
		nodes, err := loadSubscription(opts)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(nodes)
	case "generate", "update", "update-subscription":
		return update(opts)
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprintln(os.Stderr, "usage: happ-openwrt [flags] update|generate|parse")
	fmt.Fprintln(os.Stderr, "  -subscription URL   happ/proxy link or HTTP(S) subscription")
	fmt.Fprintln(os.Stderr, "  -config PATH        UCI config path")
	fmt.Fprintln(os.Stderr, "  -output PATH        sing-box config output path")
	return nil
}

func update(opts Options) error {
	if !opts.Enabled {
		return errors.New("service is disabled in UCI config")
	}
	nodes, err := loadSubscription(opts)
	if err != nil {
		return err
	}
	cfg := buildSingBoxConfig(nodes, opts)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := validateJSON(data); err != nil {
		return err
	}
	if err := atomicWrite(opts.Output, data, 0600); err != nil {
		return err
	}
	log.Printf("wrote %s with %d outbound node(s)", opts.Output, len(nodes))
	if opts.Restart {
		restartSingBox()
	}
	return nil
}

func loadSubscription(opts Options) ([]Node, error) {
	input := strings.TrimSpace(opts.Subscription)
	if input == "" {
		return nil, errors.New("missing subscription URL")
	}
	input = applyDocumentedHappParams(input, opts)
	u, err := url.Parse(input)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		body, err := download(input, opts.UserAgent)
		if err != nil {
			return nil, err
		}
		nodes, err := parseSubscriptionText(string(body), opts)
		if err != nil {
			return nil, err
		}
		return nodes, nil
	case "happ":
		return parseHappLink(input, opts)
	default:
		return parseSubscriptionText(input, opts)
	}
}

func download(raw, ua string) ([]byte, error) {
	log.Printf("downloading subscription %s", maskURL(raw))
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", "happ-openwrt/0.1")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(resp.Body, maxDownloadBytes+1)); err != nil {
		return nil, err
	}
	if buf.Len() > maxDownloadBytes {
		return nil, errors.New("subscription is too large")
	}
	return buf.Bytes(), nil
}

func parseHappLink(raw string, opts Options) ([]Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(strings.ToLower(u.Host), "crypt") || strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "crypt") {
		return nil, errors.New("encrypted happ://crypt links are not supported; private-key decryption is intentionally out of scope")
	}
	if hwid := u.Query().Get("hwid"); hwid != "" {
		if opts.HWID == "" {
			return nil, errors.New("link requires HWID; configure option hwid explicitly")
		}
		if subtlePlainCompare(hwid, opts.HWID) == false {
			return nil, errors.New("configured HWID does not match link HWID")
		}
	}
	inner := u.Query().Get("url")
	if inner == "" {
		inner = strings.TrimPrefix(u.Path, "/")
	}
	if decoded, err := url.QueryUnescape(inner); err == nil {
		inner = decoded
	}
	if inner == "" {
		return nil, errors.New("unsupported happ:// link format")
	}
	nested := opts
	nested.Subscription = inner
	return loadSubscription(nested)
}

func applyDocumentedHappParams(raw string, opts Options) string {
	if opts.InstallID == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return raw
	}
	q := u.Query()
	if q.Get("installid") == "" {
		q.Set("installid", opts.InstallID)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func parseSubscriptionText(text string, opts Options) ([]Node, error) {
	text = strings.TrimSpace(stripUnsafeText(text))
	if text == "" {
		return nil, errors.New("empty subscription")
	}
	if !containsKnownScheme(text) {
		if decoded, err := decodeBase64Loose(text); err == nil && containsKnownScheme(string(decoded)) {
			text = string(decoded)
		}
	}
	lines := splitSubscription(text)
	var nodes []Node
	for _, line := range lines {
		node, err := parseShareLink(line)
		if err != nil {
			log.Printf("skipping unsupported link: %v", err)
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, errors.New("no supported proxy nodes found")
	}
	return nodes, nil
}

func parseShareLink(raw string) (Node, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Node{}, err
	}
	switch strings.ToLower(u.Scheme) {
	case "vless":
		return parseVLESS(u)
	case "trojan":
		return parseTrojan(u)
	case "ss":
		return parseShadowsocks(raw)
	default:
		return Node{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}

func parseVLESS(u *url.URL) (Node, error) {
	port, err := parsePort(u.Port())
	if err != nil {
		return Node{}, err
	}
	q := u.Query()
	security := strings.ToLower(q.Get("security"))
	n := Node{
		Type:       "vless",
		Tag:        cleanTag(u.Fragment, "vless-"+u.Hostname()),
		Server:     u.Hostname(),
		Port:       port,
		UUID:       strings.TrimPrefix(u.User.String(), "//"),
		Flow:       q.Get("flow"),
		SNI:        firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("host")),
		TLS:        security == "tls" || security == "reality",
		AllowInsec: q.Get("allowInsecure") == "1" || strings.EqualFold(q.Get("allowInsecure"), "true"),
		Transport:  parseTransport(q),
	}
	if n.UUID == "" || n.Server == "" {
		return Node{}, errors.New("vless link missing uuid or host")
	}
	if security == "reality" {
		n.Reality = &Reality{PublicKey: q.Get("pbk"), ShortID: q.Get("sid")}
	}
	return n, nil
}

func parseTrojan(u *url.URL) (Node, error) {
	port, err := parsePort(u.Port())
	if err != nil {
		return Node{}, err
	}
	q := u.Query()
	n := Node{
		Type:       "trojan",
		Tag:        cleanTag(u.Fragment, "trojan-"+u.Hostname()),
		Server:     u.Hostname(),
		Port:       port,
		Password:   u.User.String(),
		SNI:        firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("host")),
		TLS:        true,
		AllowInsec: q.Get("allowInsecure") == "1" || strings.EqualFold(q.Get("allowInsecure"), "true"),
		Transport:  parseTransport(q),
	}
	if n.Password == "" || n.Server == "" {
		return Node{}, errors.New("trojan link missing password or host")
	}
	return n, nil
}

func parseShadowsocks(raw string) (Node, error) {
	without := strings.TrimPrefix(raw, "ss://")
	frag := ""
	if i := strings.IndexByte(without, '#'); i >= 0 {
		frag, _ = url.QueryUnescape(without[i+1:])
		without = without[:i]
	}
	if i := strings.IndexByte(without, '?'); i >= 0 {
		without = without[:i]
	}
	if !strings.Contains(without, "@") {
		decoded, err := decodeBase64Loose(without)
		if err != nil {
			return Node{}, err
		}
		without = string(decoded)
	} else {
		parts := strings.SplitN(without, "@", 2)
		if !strings.Contains(parts[0], ":") {
			decoded, err := decodeBase64Loose(parts[0])
			if err == nil {
				without = string(decoded) + "@" + parts[1]
			}
		}
	}
	u, err := url.Parse("ss://" + without)
	if err != nil {
		return Node{}, err
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return Node{}, err
	}
	method := u.User.Username()
	password, _ := u.User.Password()
	if password == "" && strings.Contains(u.User.String(), ":") {
		pair := strings.SplitN(u.User.String(), ":", 2)
		method, password = pair[0], pair[1]
	}
	n := Node{
		Type:     "shadowsocks",
		Tag:      cleanTag(frag, "ss-"+u.Hostname()),
		Server:   u.Hostname(),
		Port:     port,
		Method:   method,
		Password: password,
	}
	if n.Method == "" || n.Password == "" || n.Server == "" {
		return Node{}, errors.New("ss link missing method, password, or host")
	}
	return n, nil
}

func parseTransport(q url.Values) map[string]any {
	switch strings.ToLower(firstNonEmpty(q.Get("type"), q.Get("network"))) {
	case "ws", "websocket":
		t := map[string]any{"type": "ws"}
		if path := q.Get("path"); path != "" {
			t["path"] = path
		}
		if host := q.Get("host"); host != "" {
			t["headers"] = map[string]string{"Host": host}
		}
		return t
	case "grpc":
		t := map[string]any{"type": "grpc"}
		if service := q.Get("serviceName"); service != "" {
			t["service_name"] = service
		}
		return t
	default:
		return nil
	}
}

func buildSingBoxConfig(nodes []Node, opts Options) map[string]any {
	var outbounds []any
	var tags []string
	for i, n := range nodes {
		tag := uniqueTag(n.Tag, tags, i)
		tags = append(tags, tag)
		outbounds = append(outbounds, nodeOutbound(n, tag))
	}
	outbounds = append(outbounds,
		map[string]any{"type": "selector", "tag": "proxy", "outbounds": tags, "default": tags[0]},
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
		map[string]any{"type": "dns", "tag": "dns-out"},
	)
	rules := []any{map[string]any{"protocol": "dns", "outbound": "dns-out"}}
	final := "proxy"
	switch opts.ProxyMode {
	case "direct":
		final = "direct"
	case "rule":
		rules = append(rules,
			map[string]any{"ip_is_private": true, "outbound": "direct"},
			map[string]any{"domain_suffix": []string{"local", "lan"}, "outbound": "direct"},
		)
	}
	return map[string]any{
		"log": map[string]any{"level": "info", "timestamp": true},
		"dns": dnsConfig(opts.DNSMode),
		"inbounds": []any{map[string]any{
			"type":           "tun",
			"tag":            "tun-in",
			"interface_name": "happ0",
			"address":        []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
			"auto_route":     true,
			"strict_route":   true,
			"sniff":          true,
		}},
		"outbounds": outbounds,
		"route":     map[string]any{"rules": rules, "final": final, "auto_detect_interface": true},
	}
}

func nodeOutbound(n Node, tag string) map[string]any {
	o := map[string]any{
		"type":        n.Type,
		"tag":         tag,
		"server":      n.Server,
		"server_port": n.Port,
	}
	switch n.Type {
	case "vless":
		o["uuid"] = n.UUID
		if n.Flow != "" {
			o["flow"] = n.Flow
		}
	case "trojan":
		o["password"] = n.Password
	case "shadowsocks":
		o["method"] = n.Method
		o["password"] = n.Password
	}
	if n.TLS {
		tls := map[string]any{"enabled": true}
		if n.SNI != "" {
			tls["server_name"] = n.SNI
		}
		if n.AllowInsec {
			tls["insecure"] = true
		}
		if n.Reality != nil {
			tls["reality"] = map[string]any{"enabled": true, "public_key": n.Reality.PublicKey, "short_id": n.Reality.ShortID}
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
		}
		o["tls"] = tls
	}
	if n.Transport != nil {
		o["transport"] = n.Transport
	}
	return o
}

func dnsConfig(mode string) map[string]any {
	switch mode {
	case "local", "system":
		return map[string]any{"servers": []any{map[string]any{"tag": "local", "address": "local"}}, "strategy": "prefer_ipv4"}
	default:
		return map[string]any{"servers": []any{map[string]any{"tag": "remote", "address": "tls://1.1.1.1"}}, "final": "remote", "strategy": "prefer_ipv4"}
	}
}

func readUCIConfig(path string) (Options, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Options{}, err
	}
	var opts Options
	opts.Output = defaultOutputPath
	opts.ProxyMode = "global"
	opts.DNSMode = "remote"
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "option ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		key := parts[1]
		val := strings.Trim(strings.Join(parts[2:], " "), "'\"")
		switch key {
		case "enabled":
			opts.Enabled = val == "1" || strings.EqualFold(val, "true")
		case "subscription_url":
			opts.Subscription = val
		case "output_path":
			opts.Output = val
		case "proxy_mode":
			opts.ProxyMode = strings.ToLower(val)
		case "dns_mode":
			opts.DNSMode = strings.ToLower(val)
		case "user_agent":
			opts.UserAgent = val
		case "hwid":
			opts.HWID = val
		case "installid", "install_id":
			opts.InstallID = val
		}
	}
	return opts, nil
}

func mergeOptions(fileOpts, cliDefaults Options) Options {
	if fileOpts.Output == "" {
		fileOpts.Output = cliDefaults.Output
	}
	if fileOpts.ProxyMode == "" {
		fileOpts.ProxyMode = cliDefaults.ProxyMode
	}
	if fileOpts.DNSMode == "" {
		fileOpts.DNSMode = cliDefaults.DNSMode
	}
	return fileOpts
}

func validateJSON(data []byte) error {
	var v any
	return json.Unmarshal(data, &v)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".happ-openwrt-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func restartSingBox() {
	if _, err := exec.LookPath("/etc/init.d/sing-box"); err == nil {
		_ = exec.Command("/etc/init.d/sing-box", "restart").Run()
		return
	}
	_ = exec.Command("service", "sing-box", "restart").Run()
}

func splitSubscription(text string) []string {
	replacer := strings.NewReplacer("\r", "\n", "\t", "\n", " ", "\n")
	parts := strings.Split(replacer.Replace(text), "\n")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		if containsKnownScheme(p) {
			out = append(out, p)
		}
	}
	return out
}

func containsKnownScheme(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "vless://") || strings.Contains(l, "trojan://") || strings.Contains(l, "ss://")
}

func stripUnsafeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		if r < 32 {
			return -1
		}
		return r
	}, s)
}

func decodeBase64Loose(s string) ([]byte, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", ""))
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(padBase64(s)); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(padBase64(s))
}

func padBase64(s string) string {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 || p > 65535 {
		return 0, errors.New("invalid port")
	}
	return p, nil
}

func cleanTag(tag, fallback string) string {
	if tag == "" {
		tag = fallback
	}
	if decoded, err := url.QueryUnescape(tag); err == nil {
		tag = decoded
	}
	tag = regexp.MustCompile(`[^A-Za-z0-9._ -]+`).ReplaceAllString(tag, "-")
	tag = strings.TrimSpace(tag)
	if len(tag) > 48 {
		tag = tag[:48]
	}
	if tag == "" {
		tag = "proxy"
	}
	return tag
}

func uniqueTag(tag string, existing []string, i int) string {
	seen := map[string]bool{}
	for _, e := range existing {
		seen[e] = true
	}
	if !seen[tag] {
		return tag
	}
	return fmt.Sprintf("%s-%d", tag, i+1)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func maskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	u.User = nil
	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			lk := strings.ToLower(key)
			if strings.Contains(lk, "token") || strings.Contains(lk, "key") || strings.Contains(lk, "uuid") || strings.Contains(lk, "pass") || strings.Contains(lk, "id") {
				q.Set(key, "***")
			}
		}
		u.RawQuery = q.Encode()
	}
	if u.Fragment != "" {
		u.Fragment = "..."
	}
	return u.String()
}

func subtlePlainCompare(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return bytes.Equal(ha[:], hb[:])
}
