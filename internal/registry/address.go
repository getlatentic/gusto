package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/proxy"
)

// Resolver is how a name becomes addresses; a test gives its own.
type Resolver func(ctx context.Context, host string) ([]netip.Addr, error)

func systemResolver(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// SetResolver changes how names are resolved, which a test does.
func (r *Registry) SetResolver(resolve Resolver) { r.resolve = resolve }

// checkAddress says whether a provider may be at this address, and gives it as it is kept. Where a provider is
// is where the organisation's key is sent, from a machine that other organisations' servers are on too, so an
// address that is this machine, a private network or a cloud's metadata service is refused unless the operator
// allowed it. The proxy checks again when it connects, since a name can change.
func (r *Registry) checkAddress(address string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	switch {
	case err != nil || parsed.Host == "" || parsed.Hostname() == "":
		return "", errors.New("that is not an address")
	case parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "":
		return "", errors.New("an address is a scheme, a host and perhaps a path, and nothing else")
	case len(address) > 300:
		return "", errors.New("that address is too long")
	case parsed.Scheme != "https" && !(parsed.Scheme == "http" && r.Rules().PrivateAddresses):
		return "", errors.New("a provider is reached over https")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if r.Rules().PrivateAddresses {
		return parsed.String(), nil
	}
	if err := r.publicHost(parsed.Hostname()); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func (r *Registry) publicHost(host string) error {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".internal") {
		return fmt.Errorf("%s is not a public address", host)
	}
	addresses, err := r.addressesOf(host)
	if err != nil {
		return fmt.Errorf("%s could not be looked up", host)
	}
	for _, address := range addresses {
		if !proxy.Public(address.Unmap()) {
			return fmt.Errorf("%s is at %s, which is not a public address", host, address.Unmap())
		}
	}
	return nil
}

func (r *Registry) addressesOf(host string) ([]netip.Addr, error) {
	if literal, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{literal}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	found, err := r.resolve(ctx, host)
	if err == nil && len(found) == 0 {
		err = errors.New("no address")
	}
	return found, err
}
