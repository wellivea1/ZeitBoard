package portal

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Behind a reverse proxy every request arrives from the proxy's own address,
// so the per-source limits counted every visitor as one: two open pages asking
// once a minute spent the hourly allowance for everybody.
//
// The address a proxy reports in X-Forwarded-For is taken only when the
// request really came from a proxy the operator named. From anywhere else the
// header is a visitor's own words, and trusting it would let anyone choose a
// fresh address for each request and never be limited at all.

// ReadLimitThroughProxyPerHour caps everything arriving through one trusted
// proxy, whatever addresses it reports. A proxy that forwards a header it did
// not check must not turn the per-visitor limit into no limit.
const ReadLimitThroughProxyPerHour = 20 * ReadLimitPerHour

// ParseTrustedProxies reads the operator's list: addresses or CIDR ranges.
func ParseTrustedProxies(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is neither an address nor a CIDR range", value)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

func trusted(addr netip.Addr, proxies []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, proxy := range proxies {
		if proxy.Contains(addr) {
			return true
		}
	}
	return false
}

func remoteIP(remoteAddr string) (netip.Addr, bool) {
	host := strings.TrimSpace(remoteAddr)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	addr, err := netip.ParseAddr(host)
	return addr.Unmap(), err == nil
}

// visitorAddr is the address to count a request against, and whether it came
// through a trusted proxy.
//
// From a trusted proxy it is the last address in X-Forwarded-For that is not
// itself a trusted proxy: read from the right, because the proxy appends what
// it saw and everything to the left of that is whatever the visitor sent. A
// header that is missing or names nothing usable leaves the proxy's own
// address, which limits too much and never too little.
func visitorAddr(r *http.Request, proxies []netip.Prefix) (string, bool) {
	peer, ok := remoteIP(r.RemoteAddr)
	if !ok || len(proxies) == 0 || !trusted(peer, proxies) {
		return r.RemoteAddr, false
	}
	var hops []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(line, ",")...)
	}
	for index := len(hops) - 1; index >= 0; index-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if err != nil {
			// A proxy that wrote something unreadable is not to be read past.
			break
		}
		if !trusted(addr, proxies) {
			return addr.Unmap().String(), true
		}
	}
	return r.RemoteAddr, true
}
