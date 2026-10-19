// dns points a demo host's name at the host, or takes it away: the name and every name
// under it, as A records that do not go through Cloudflare's proxy. The host runs it
// when it starts, so nothing else needs the token.
//
//	dns -zone example.com -name gusto.example.com -ip 203.0.113.7
//	dns -zone example.com -name gusto.example.com -remove
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	zone := flag.String("zone", "", "the Cloudflare zone, such as example.com")
	name := flag.String("name", "", "the name organisations are served under")
	ip := flag.String("ip", "", "the host's public IPv4 address")
	remove := flag.Bool("remove", false, "remove the records instead of pointing them")
	tokenFile := flag.String("token-file", "", "a file holding the Cloudflare API token that may edit this zone's DNS")
	api := flag.String("api", "", "Cloudflare's API address, for a test")
	flag.Parse()
	if err := run(*api, *zone, *name, *ip, *tokenFile, *remove); err != nil {
		fmt.Fprintln(os.Stderr, "dns:", err)
		os.Exit(1)
	}
}

func run(api, zone, name, ip, tokenFile string, remove bool) error {
	if name == "" {
		return errNoName
	}
	if zone == "" || (name != zone && !strings.HasSuffix(name, "."+zone)) {
		return fmt.Errorf("-name %q is not in -zone %q", name, zone)
	}
	if parsed := net.ParseIP(ip); !remove && (parsed == nil || parsed.To4() == nil) {
		return fmt.Errorf("-ip %q is not an IPv4 address", ip)
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		return err
	}
	client := newZoneClient(api, strings.TrimSpace(string(token)), zone)
	for _, each := range []string{name, "*." + name} {
		if remove {
			err = client.remove(each)
		} else {
			err = client.point(each, ip)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
