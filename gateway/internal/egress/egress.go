// Package egress controls where this process is allowed to make outbound
// requests.
//
// The removed webhook subsystem accepted an arbitrary URL from a request body
// and fetched it. The baseline audit reproduced the consequence directly: a
// POST to /webhooks/test with the cloud metadata address issued a request to
// 169.254.169.254, and nothing in the code stopped it. On a cloud instance that
// address returns instance credentials.
//
// That subsystem is gone, but the class of defect is not: this process still
// fetches two operator-configured URLs -- the identity provider's JWKS document
// and the AI tier. Both are attacker-relevant if an attacker can influence
// configuration, and both are the natural place for a future feature to accept
// a caller-supplied URL. The control belongs here, once, rather than in each
// call site.
//
// The design decision that matters: this package resolves a hostname, validates
// every address it resolves to, and then dials the validated address. It does
// not validate a hostname and then hand the hostname to the dialer. That
// distinction is the whole of DNS rebinding -- a name that resolves to a public
// address during a check and to 127.0.0.1 a moment later when the connection is
// actually made.
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNotAllowed covers every refusal: a host off the allowlist, a
	// disallowed scheme, an address in a blocked range. The message names the
	// reason because the caller is an operator reading a log, not an attacker
	// -- these URLs come from configuration, not from a request.
	ErrNotAllowed = errors.New("egress denied by policy")

	// ErrTooManyRedirects is separate because it is a bound, not a policy
	// violation.
	ErrTooManyRedirects = errors.New("too many redirects")

	// ErrResponseTooLarge is returned when a response exceeds the byte limit.
	ErrResponseTooLarge = errors.New("response exceeds the configured size limit")

	// ErrRateLimited is returned when a destination's request budget is spent.
	ErrRateLimited = errors.New("egress rate limit exceeded for this destination")
)

// Policy is the complete set of outbound restrictions. The zero Policy allows
// nothing, which is the correct default for a financial system: a missing
// configuration must not silently open the network.
type Policy struct {
	// AllowedHosts are exact hostnames, compared case-insensitively after
	// trailing-dot normalisation. A wildcard is not supported deliberately:
	// "*.example.com" invites a subdomain-takeover to become an SSRF.
	AllowedHosts []string

	// RequireHTTPS refuses plaintext. Set for anything carrying a credential.
	RequireHTTPS bool

	// AllowPrivateAddresses disables the address-range checks.
	//
	// It exists for the local-demo profile and for tests that use a loopback
	// server, and it is a named field rather than an inferred condition so that
	// enabling it is visible in configuration and in a diff.
	AllowPrivateAddresses bool

	// Timeout bounds the whole request including redirects.
	Timeout time.Duration

	// MaxResponseBytes bounds what is read from a response body. A remote host
	// that streams indefinitely is a memory-exhaustion vector.
	MaxResponseBytes int64

	// MaxRedirects bounds redirect chains. Every hop is re-validated against
	// the full policy; a redirect is a fresh destination, not a continuation.
	MaxRedirects int

	// RequestsPerMinute bounds calls per destination host. Zero means
	// unlimited, which is appropriate for a JWKS endpoint fetched at startup
	// and wrong for anything driven by request traffic.
	RequestsPerMinute int
}

// Guard enforces a Policy.
type Guard struct {
	policy Policy

	// resolver is injectable so tests can supply the addresses a hostname
	// resolves to without depending on DNS or on a hosts file.
	resolver func(ctx context.Context, host string) ([]net.IP, error)

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	windowStart time.Time
	count       int
}

// New builds a Guard.
func New(p Policy) *Guard {
	g := &Guard{policy: p, buckets: map[string]*bucket{}}
	g.resolver = func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}
	return g
}

// SetResolver replaces address resolution. Tests use it to present a hostname
// that resolves to a blocked address, and to model DNS rebinding by returning
// different addresses on successive calls.
func (g *Guard) SetResolver(fn func(ctx context.Context, host string) ([]net.IP, error)) {
	g.resolver = fn
}

// normalizeHost lowercases and strips a trailing dot, so "EXAMPLE.COM." and
// "example.com" cannot be treated as different hosts -- one of which is on an
// allowlist and one of which is not.
func normalizeHost(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
}

// CheckURL validates a destination before any connection is attempted.
//
// It is the pre-flight check. It is not sufficient on its own, because the
// address a hostname resolves to can change between this check and the dial;
// the dialer performs the address checks again on the address it will actually
// connect to.
func (g *Guard) CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a valid URL", ErrNotAllowed, raw)
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if g.policy.RequireHTTPS {
			return nil, fmt.Errorf("%w: scheme http is refused; https is required", ErrNotAllowed)
		}
	default:
		// file://, gopher://, ftp:// and dict:// are classic SSRF escalations.
		return nil, fmt.Errorf("%w: scheme %q is not permitted", ErrNotAllowed, u.Scheme)
	}

	if u.User != nil {
		// Credentials in a URL end up in logs, in Referer headers and in error
		// messages. There is no case where this is the right way to carry one.
		return nil, fmt.Errorf("%w: the URL embeds credentials in its userinfo", ErrNotAllowed)
	}

	host := normalizeHost(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("%w: the URL has no host", ErrNotAllowed)
	}
	if !g.hostAllowed(host) {
		return nil, fmt.Errorf("%w: host %q is not on the allowlist", ErrNotAllowed, host)
	}
	return u, nil
}

func (g *Guard) hostAllowed(host string) bool {
	for _, allowed := range g.policy.AllowedHosts {
		if normalizeHost(allowed) == host {
			return true
		}
	}
	return false
}

// CheckAddress rejects an IP the process must never connect to.
//
// The unmapping on the first line is load-bearing: ::ffff:127.0.0.1 is
// loopback written as an IPv6 address, and a check that inspects the IPv6 form
// without unmapping treats it as an ordinary global address.
func (g *Guard) CheckAddress(ip net.IP) error {
	if g.policy.AllowPrivateAddresses {
		return nil
	}
	if ip == nil {
		return fmt.Errorf("%w: no address", ErrNotAllowed)
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	switch {
	case ip.IsLoopback():
		return fmt.Errorf("%w: %s is loopback", ErrNotAllowed, ip)
	case ip.IsUnspecified():
		// 0.0.0.0 is routed to localhost by many stacks.
		return fmt.Errorf("%w: %s is unspecified", ErrNotAllowed, ip)
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		// 169.254.0.0/16 contains 169.254.169.254, the cloud metadata address
		// the removed webhook tester actually reached.
		return fmt.Errorf("%w: %s is link-local; this range contains the cloud instance metadata service", ErrNotAllowed, ip)
	case ip.IsPrivate():
		return fmt.Errorf("%w: %s is a private address", ErrNotAllowed, ip)
	case ip.IsMulticast():
		return fmt.Errorf("%w: %s is multicast", ErrNotAllowed, ip)
	case ip.IsInterfaceLocalMulticast():
		return fmt.Errorf("%w: %s is interface-local multicast", ErrNotAllowed, ip)
	}

	for _, blocked := range blockedNets {
		if blocked.net.Contains(ip) {
			return fmt.Errorf("%w: %s is in %s (%s)", ErrNotAllowed, ip, blocked.net.String(), blocked.why)
		}
	}
	return nil
}

type namedNet struct {
	net *net.IPNet
	why string
}

// blockedNets covers ranges the standard library's predicates do not.
var blockedNets = func() []namedNet {
	specs := []struct{ cidr, why string }{
		{"100.64.0.0/10", "carrier-grade NAT"},
		{"192.0.0.0/24", "IETF protocol assignments"},
		{"192.0.2.0/24", "documentation"},
		{"198.18.0.0/15", "benchmarking"},
		{"198.51.100.0/24", "documentation"},
		{"203.0.113.0/24", "documentation"},
		{"240.0.0.0/4", "reserved"},
		{"255.255.255.255/32", "broadcast"},
		// Alibaba and Oracle metadata endpoints live on 100.100.x and
		// 192.0.0.192 respectively; both are already covered above. NAT64 is
		// listed because an embedded IPv4 address can carry a blocked target
		// through an IPv6 literal.
		{"64:ff9b::/96", "NAT64, which can embed a blocked IPv4 address"},
		{"2001:db8::/32", "documentation"},
		{"fec0::/10", "deprecated site-local"},
		{"::/128", "unspecified"},
	}
	out := make([]namedNet, 0, len(specs))
	for _, s := range specs {
		_, n, err := net.ParseCIDR(s.cidr)
		if err != nil {
			panic("egress: bad blocked CIDR " + s.cidr)
		}
		out = append(out, namedNet{net: n, why: s.why})
	}
	return out
}()

// allow applies the per-host rate limit.
func (g *Guard) allow(host string, now time.Time) error {
	if g.policy.RequestsPerMinute <= 0 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	b, ok := g.buckets[host]
	if !ok || now.Sub(b.windowStart) >= time.Minute {
		g.buckets[host] = &bucket{windowStart: now, count: 1}
		return nil
	}
	if b.count >= g.policy.RequestsPerMinute {
		return fmt.Errorf("%w: %s", ErrRateLimited, host)
	}
	b.count++
	return nil
}

// dialContext resolves, validates every resolved address, and connects to a
// validated address.
//
// Connecting to the address rather than re-resolving the name is what closes
// the DNS rebinding window. A second resolution between check and connect is
// exactly the attack.
func (g *Guard) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotAllowed, err)
	}
	host = normalizeHost(host)

	if !g.hostAllowed(host) {
		return nil, fmt.Errorf("%w: host %q is not on the allowlist", ErrNotAllowed, host)
	}
	if err := g.allow(host, time.Now()); err != nil {
		return nil, err
	}

	// A literal address skips resolution but not validation.
	var ips []net.IP
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	} else {
		ips, err = g.resolver(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%w: %s resolved to no addresses", ErrNotAllowed, host)
	}

	// Every address must pass. Connecting to the first acceptable one while
	// others are blocked would let an attacker who controls DNS present one
	// good address alongside the target and win on a retry.
	for _, ip := range ips {
		if err := g.CheckAddress(ip); err != nil {
			return nil, err
		}
	}

	var lastErr error
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// Client returns an http.Client that enforces the policy on every request,
// including every redirect hop.
func (g *Guard) Client() *http.Client {
	transport := &http.Transport{
		DialContext: g.dialContext,
		// Bounded, because an idle pool to a blocked destination would outlive
		// a policy change.
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	timeout := g.policy.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			max := g.policy.MaxRedirects
			if len(via) > max {
				return fmt.Errorf("%w: %d exceeds the limit of %d", ErrTooManyRedirects, len(via), max)
			}
			// A redirect is a new destination. Re-running the full URL check is
			// what stops the classic bypass: an allowlisted host that answers
			// with a 302 to the metadata service.
			if _, err := g.CheckURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect to %s: %w", req.URL.Redacted(), err)
			}
			return nil
		},
	}
}

// Get performs a bounded GET through the guarded client.
//
// It returns the body already limited to MaxResponseBytes and reports
// ErrResponseTooLarge rather than silently truncating: a truncated JWKS
// document that happens to parse would be a key set with keys missing.
func (g *Guard) Get(ctx context.Context, raw string) (*http.Response, []byte, error) {
	if _, err := g.CheckURL(raw); err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := g.Client().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	limit := g.policy.MaxResponseBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	// Read one byte past the limit so exceeding it is detectable rather than
	// indistinguishable from a body that is exactly the limit.
	body, err := readAtMost(resp, limit+1)
	if err != nil {
		return nil, nil, err
	}
	if int64(len(body)) > limit {
		return nil, nil, fmt.Errorf("%w: more than %d bytes", ErrResponseTooLarge, limit)
	}
	return resp, body, nil
}

func readAtMost(resp *http.Response, n int64) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	var total int64
	for total < n {
		read, err := resp.Body.Read(tmp)
		if read > 0 {
			remaining := n - total
			if int64(read) > remaining {
				read = int(remaining)
			}
			buf = append(buf, tmp[:read]...)
			total += int64(read)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			break
		}
	}
	return buf, nil
}
