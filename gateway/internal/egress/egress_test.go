package egress

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The address ranges an outbound request must never reach.
//
// 169.254.169.254 is the case the Prompt 00 baseline reproduced against the
// running system: POST /webhooks/test issued a request to it and nothing
// stopped the request.
func TestBlockedAddressRanges(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"anything"}})

	blocked := map[string]string{
		"127.0.0.1":              "loopback",
		"127.1.2.3":              "loopback, not only .0.1",
		"0.0.0.0":                "unspecified, routed to localhost by many stacks",
		"169.254.169.254":        "AWS/Azure/GCP instance metadata",
		"169.254.170.2":          "ECS task metadata",
		"100.100.100.200":        "Alibaba metadata",
		"192.0.0.192":            "Oracle metadata",
		"10.0.0.1":               "private",
		"172.16.5.4":             "private",
		"192.168.1.1":            "private",
		"100.64.0.1":             "carrier-grade NAT",
		"224.0.0.1":              "multicast",
		"255.255.255.255":        "broadcast",
		"::1":                    "IPv6 loopback",
		"fe80::1":                "IPv6 link-local",
		"fc00::1":                "IPv6 unique-local",
		"::":                     "IPv6 unspecified",
		"::ffff:127.0.0.1":       "IPv4-mapped loopback -- blocked only if the check unmaps first",
		"::ffff:169.254.169.254": "IPv4-mapped metadata address",
		"64:ff9b::7f00:1":        "NAT64-embedded loopback",
	}

	for addr, why := range blocked {
		ip := net.ParseIP(addr)
		if ip == nil {
			t.Fatalf("test fixture %q is not a valid IP", addr)
		}
		if err := g.CheckAddress(ip); err == nil {
			t.Errorf("%s (%s) was permitted", addr, why)
		} else if !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s returned %v, want ErrNotAllowed", addr, err)
		}
	}
}

func TestOrdinaryPublicAddressesArePermitted(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"anything"}})
	for _, addr := range []string{"93.184.216.34", "8.8.8.8", "2606:2800:220:1:248:1893:25c8:1946"} {
		if err := g.CheckAddress(net.ParseIP(addr)); err != nil {
			t.Errorf("%s was blocked but is an ordinary public address: %v", addr, err)
		}
	}
}

// A hostname off the allowlist is refused regardless of where it resolves.
func TestAllowlistIsMandatory(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"idp.example.com"}, RequireHTTPS: true})

	if _, err := g.CheckURL("https://idp.example.com/.well-known/jwks.json"); err != nil {
		t.Errorf("the allowlisted host was refused: %v", err)
	}
	for _, raw := range []string{
		"https://evil.example.com/jwks",
		"https://idp.example.com.evil.com/jwks", // suffix confusion
		"https://sub.idp.example.com/jwks",      // no implicit wildcard
	} {
		if _, err := g.CheckURL(raw); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s was permitted: %v", raw, err)
		}
	}

	// The zero policy allows nothing.
	if _, err := New(Policy{}).CheckURL("https://idp.example.com/"); !errors.Is(err, ErrNotAllowed) {
		t.Error("an empty policy permitted a request; the default must deny")
	}
}

// Host comparison must not be case- or trailing-dot-sensitive in a way that
// makes two spellings of one host behave differently.
func TestHostMatchingIsNormalised(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"IDP.Example.COM"}})
	for _, raw := range []string{
		"https://idp.example.com/x",
		"https://IDP.EXAMPLE.COM/x",
		"https://idp.example.com./x",
	} {
		if _, err := g.CheckURL(raw); err != nil {
			t.Errorf("%s was refused despite matching the allowlist: %v", raw, err)
		}
	}
}

// Schemes other than http(s) are escalation paths: file:// reads local files,
// gopher:// can forge arbitrary TCP payloads.
func TestOnlyHTTPSchemesArePermitted(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"idp.example.com", "etc"}})
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://idp.example.com/_GET",
		"ftp://idp.example.com/x",
		"dict://idp.example.com:11211/stat",
	} {
		if _, err := g.CheckURL(raw); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s was permitted", raw)
		}
	}
}

func TestHTTPSCanBeRequired(t *testing.T) {
	strict := New(Policy{AllowedHosts: []string{"idp.example.com"}, RequireHTTPS: true})
	if _, err := strict.CheckURL("http://idp.example.com/jwks"); !errors.Is(err, ErrNotAllowed) {
		t.Error("plaintext http was permitted where https is required")
	}

	relaxed := New(Policy{AllowedHosts: []string{"idp.example.com"}})
	if _, err := relaxed.CheckURL("http://idp.example.com/jwks"); err != nil {
		t.Errorf("http was refused where it is permitted: %v", err)
	}
}

// Credentials in a URL reach logs, Referer headers and error messages.
func TestURLUserinfoIsRefused(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"idp.example.com"}})
	if _, err := g.CheckURL("https://user:password@idp.example.com/jwks"); !errors.Is(err, ErrNotAllowed) {
		t.Error("a URL carrying credentials in its userinfo was permitted")
	}
}

// Decimal, octal and hexadecimal spellings of an IP address are the standard
// way of writing a blocked address so that a string check misses it.
func TestAlternateIPEncodingsAreRefused(t *testing.T) {
	// The allowlist is the first defence: these spellings are not the
	// configured hostname, so they never reach resolution.
	g := New(Policy{AllowedHosts: []string{"idp.example.com"}})
	for _, host := range []string{
		"2130706433", // decimal 127.0.0.1
		"0x7f000001", // hex 127.0.0.1
		"0177.0.0.1", // octal 127.0.0.1
		"127.1",      // short form 127.0.0.1
		"[::ffff:127.0.0.1]",
		"2852039166", // decimal 169.254.169.254
	} {
		if _, err := g.CheckURL("https://" + host + "/x"); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("https://%s/x was permitted by the allowlist check", host)
		}
	}

	// And the address check is the second: even with the spelling allowlisted,
	// the address it denotes is still refused at dial time.
	permissive := New(Policy{AllowedHosts: []string{"127.1", "0x7f000001", "2130706433"}})
	for _, host := range []string{"127.1", "0x7f000001", "2130706433"} {
		_, err := permissive.dialContext(context.Background(), "tcp", host+":443")
		if err == nil {
			t.Errorf("dialing %s succeeded; a blocked address was reached", host)
		}
	}
}

// DNS rebinding: a name that resolves to a public address when it is checked
// and to a blocked address by the time a connection is made.
//
// The defence has two halves and this test covers the first. The address check
// runs inside the dialer, on the resolution the dialer itself performs, so a
// name that has rebound since any earlier check is refused at connect time
// rather than passing on the strength of a stale answer.
//
// The second half -- that no window exists between that check and the
// connection, because the guard dials the validated address literal rather than
// re-resolving the name -- is covered by
// TestTheDialerConnectsToTheValidatedAddress below.
func TestDNSRebindingIsRefusedAtConnectTime(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"rebind.example.com"}})

	var rebound bool
	g.SetResolver(func(ctx context.Context, host string) ([]net.IP, error) {
		if rebound {
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	// The pre-flight check passes: it validates the scheme and the allowlist,
	// which the attacker's hostname satisfies.
	if _, err := g.CheckURL("https://rebind.example.com/x"); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// While the name resolves benignly the dial is not refused by policy. The
	// connection itself may or may not succeed in this environment, which is
	// why only the policy outcome is asserted.
	if _, err := g.dialContext(context.Background(), "tcp", "rebind.example.com:443"); errors.Is(err, ErrNotAllowed) {
		t.Fatalf("a benign resolution was refused by policy: %v", err)
	}

	// The name now rebinds to the metadata service.
	rebound = true
	_, err := g.dialContext(context.Background(), "tcp", "rebind.example.com:443")
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("the rebound name was dialed: %v", err)
	}
	if !strings.Contains(err.Error(), "169.254.169.254") {
		t.Errorf("the refusal does not name the address it refused: %v", err)
	}
}

// The dialer must connect to the address it validated, not re-resolve the name.
//
// A second resolution between the check and the connection is the whole of DNS
// rebinding. This proves there is no second resolution: the resolver is called
// once, and the connection lands on the address it returned even though that
// address has no relationship to the hostname in DNS.
func TestTheDialerConnectsToTheValidatedAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("reached"))
	}))
	defer srv.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}

	var resolutions int
	g := New(Policy{
		AllowedHosts:          []string{"pinned.example.com"},
		AllowPrivateAddresses: true, // the test server is on loopback
		MaxResponseBytes:      1 << 10,
	})
	g.SetResolver(func(ctx context.Context, host string) ([]net.IP, error) {
		resolutions++
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})

	_, body, err := g.Get(context.Background(), "http://pinned.example.com:"+port+"/")
	if err != nil {
		t.Fatalf("the guarded request failed: %v", err)
	}
	if string(body) != "reached" {
		t.Errorf("body = %q", body)
	}
	if resolutions != 1 {
		t.Errorf("the name was resolved %d times; each extra resolution is a rebinding window", resolutions)
	}
}

// A hostname that resolves to a mix of permitted and blocked addresses must be
// refused entirely. Connecting to the acceptable one lets an attacker who
// controls DNS win on a retry.
func TestMixedResolutionIsRefusedEntirely(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"mixed.example.com"}})
	g.SetResolver(func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{
			net.ParseIP("93.184.216.34"),
			net.ParseIP("169.254.169.254"),
		}, nil
	})

	_, err := g.dialContext(context.Background(), "tcp", "mixed.example.com:443")
	if !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a host resolving to both a permitted and a blocked address was dialed: %v", err)
	}
}

func TestHostResolvingToNothingIsRefused(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"empty.example.com"}})
	g.SetResolver(func(ctx context.Context, host string) ([]net.IP, error) { return nil, nil })

	if _, err := g.dialContext(context.Background(), "tcp", "empty.example.com:443"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a host resolving to no addresses was not refused: %v", err)
	}
}

// A redirect is a new destination. An allowlisted host answering 302 to the
// metadata service is the classic bypass.
func TestRedirectToABlockedDestinationIsRefused(t *testing.T) {
	var target *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer target.Close()

	host, _, _ := net.SplitHostPort(strings.TrimPrefix(target.URL, "http://"))
	g := New(Policy{
		AllowedHosts: []string{host},
		// The test server is on loopback, so private addresses must be
		// permitted for the first hop to succeed at all. The redirect is still
		// refused, by the allowlist check on the new destination.
		AllowPrivateAddresses: true,
		MaxRedirects:          5,
	})

	_, _, err := g.Get(context.Background(), target.URL)
	if err == nil {
		t.Fatal("a redirect to the metadata service was followed")
	}
	if !strings.Contains(err.Error(), "169.254.169.254") {
		t.Errorf("the refusal does not name the destination it refused: %v", err)
	}
}

func TestRedirectChainsAreBounded(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	defer srv.Close()

	host, _, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	g := New(Policy{AllowedHosts: []string{host}, AllowPrivateAddresses: true, MaxRedirects: 2})

	_, _, err := g.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("an unbounded redirect loop was followed")
	}
	if !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("got %v, want a redirect-limit error", err)
	}
}

// A remote host that streams indefinitely is a memory-exhaustion vector.
func TestResponseSizeIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 4096)
		for i := 0; i < 64; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer srv.Close()

	host, _, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	g := New(Policy{
		AllowedHosts: []string{host}, AllowPrivateAddresses: true,
		MaxResponseBytes: 8192,
	})

	_, _, err := g.Get(context.Background(), srv.URL)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("got %v, want ErrResponseTooLarge", err)
	}
}

func TestAResponseWithinTheLimitIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	host, _, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	g := New(Policy{AllowedHosts: []string{host}, AllowPrivateAddresses: true, MaxResponseBytes: 1 << 20})

	resp, body, err := g.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("a permitted request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d", resp.StatusCode)
	}
	if string(body) != `{"keys":[]}` {
		t.Errorf("body = %q", body)
	}
}

func TestPerHostRateLimit(t *testing.T) {
	g := New(Policy{AllowedHosts: []string{"idp.example.com"}, RequestsPerMinute: 3})
	now := time.Now()

	for i := 0; i < 3; i++ {
		if err := g.allow("idp.example.com", now); err != nil {
			t.Fatalf("request %d was limited early: %v", i+1, err)
		}
	}
	if err := g.allow("idp.example.com", now); !errors.Is(err, ErrRateLimited) {
		t.Errorf("the fourth request was not limited: %v", err)
	}

	// A different host has its own budget.
	if err := g.allow("other.example.com", now); err != nil {
		t.Errorf("a different host shared the budget: %v", err)
	}
	// The window rolls.
	if err := g.allow("idp.example.com", now.Add(time.Minute+time.Second)); err != nil {
		t.Errorf("the budget did not reset after the window: %v", err)
	}
}

// The private-address escape hatch must be an explicit, named choice.
func TestPrivateAddressesRequireAnExplicitOptIn(t *testing.T) {
	strict := New(Policy{AllowedHosts: []string{"h"}})
	if err := strict.CheckAddress(net.ParseIP("127.0.0.1")); err == nil {
		t.Error("loopback was permitted by default")
	}

	permissive := New(Policy{AllowedHosts: []string{"h"}, AllowPrivateAddresses: true})
	if err := permissive.CheckAddress(net.ParseIP("127.0.0.1")); err != nil {
		t.Errorf("loopback was refused despite the explicit opt-in: %v", err)
	}
}
