package middleware

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8",
	"::1",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
	// Cloudflare IPs
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
	// QYYCDN IPs
	"164.92.107.0/24",
	"4.193.142.0/24",
	"154.36.166.0/24",
	"159.65.7.0/24",
	"3.35.166.0/24",
	"64.227.96.0/24",
	"137.184.82.0/24",
	"103.143.81.0/24",
	"4.193.186.0/24",
	"70.36.96.0/24",
	"206.189.146.0/24",
	"13.250.19.0/24",
	"23.98.32.0/24",
	"54.248.3.0/24",
	"54.199.123.0/24",
	"209.38.255.0/24",
	"143.20.156.0/24",
	"20.2.91.0/24",
	"18.143.94.0/24",
	"54.250.155.0/24",
	"13.212.110.0/24",
	"46.101.215.0/24",
	"168.110.202.0/24",
	"52.194.183.0/24",
	"200.180.165.0/24",
	"54.199.164.0/24",
	"54.199.68.0/24",
	"43.207.3.0/24",
	"54.255.186.0/24",
	"2406:da14:1886:2a00::/64",
	"2406:da18:594:4000::/64",
	"2406:da14:864:5a00::/64",
	"2602:f864:214:a::/64",
	"2400:6180:0:d2::/64",
	"2406:da14:159b:2f00::/64",
	"2406:da14:180:ff00::/64",
	"2406:da18:511:b300::/64",
	"2001:df1:7880:100::/64",
	"2a0e:97c0:3f0:1::/64",
	"2602:f864:214:7::/64",
	"2406:da14:1f52:bb00::/64",
	"2406:da18:d3f:f800::/64",
	"2406:da14:18f3:cf00::/64",
	"2406:da14:ea7:8400::/64",
	"2406:da18:499:6900::/64",
	"2a03:b0c0:3:f0::/64",
	"2604:a880:4:1d0::/64",
}

func ConfigureTrustedProxies(engine *gin.Engine) error {
	rawTrustedProxies := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if rawTrustedProxies == "" {
		log.Print("WARNING: TRUSTED_PROXIES is unset or blank; trusting loopback, RFC 1918, and IPv6 ULA proxy addresses for compatibility. Set TRUSTED_PROXIES=none to trust no proxies, or configure explicit proxy IPs/CIDRs to extend these defaults.")
		return engine.SetTrustedProxies(defaultTrustedProxyCIDRs)
	}
	if strings.EqualFold(rawTrustedProxies, "none") {
		return engine.SetTrustedProxies(nil)
	}

	trustedProxies := append([]string(nil), defaultTrustedProxyCIDRs...)
	configuredProxyCount := 0
	for part := range strings.SplitSeq(rawTrustedProxies, ",") {
		trustedProxy := strings.TrimSpace(part)
		if trustedProxy == "" {
			continue
		}
		if strings.EqualFold(trustedProxy, "none") {
			return errors.New("TRUSTED_PROXIES=none must be used alone")
		}
		trustedProxies = append(trustedProxies, trustedProxy)
		configuredProxyCount++
	}
	if configuredProxyCount == 0 {
		return errors.New("TRUSTED_PROXIES does not contain an IP address or CIDR")
	}
	if err := engine.SetTrustedProxies(trustedProxies); err != nil {
		return fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}
	return nil
}
