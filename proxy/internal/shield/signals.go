package shield

import (
	_ "embed"
	"math"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
)

//go:embed data/ai-crawlers.txt
var aiCrawlersRaw string

//go:embed data/scanners.txt
var scannersRaw string

func loadList(raw string) []string {
	var out []string
	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, strings.ToLower(l))
	}
	return out
}

func fastExp2(x float64) float64 { return math.Exp2(x) }

// honeypot path fragments matched case-insensitively against the request path.
var defaultHoneypots = []string{
	"/.env", "/.git/", "/.svn/", "/.hg/", "/.ds_store",
	"/wp-login.php", "/wp-admin", "/wp-content/plugins", "/xmlrpc.php",
	"/phpmyadmin", "/pma/", "/adminer", "/cgi-bin/", "/vendor/phpunit",
	"/actuator", "/server-status", "/.aws/credentials", "/id_rsa",
	"/backup.", "/boaform", "/hnap1", "/solr/admin", "/jmx-console",
	"/manager/html", "/console", "/owa/auth", "/autodiscover/autodiscover.xml",
	"/ecp/", "/remote/login", "/+cscoe+/", "/vpn/index.html",
	"/config.json", "/config.yml", "/secrets", "/credentials",
}

var (
	reTraversal = regexp.MustCompile(`\.\./|\.\.\\|%2e%2e`)
	reNullByte  = regexp.MustCompile(`%00|\x00`)
	reJNDI      = regexp.MustCompile(`(?i)\$\{jndi:`)
	reShellMeta = regexp.MustCompile(`(?i)(;|\||&&|\$\(|` + "`" + `)(wget|curl|bash|sh|nc|cat\s+/etc)`)
)

// isHoneypotPath reports whether path hits a tripwire.
func isHoneypotPath(p string, extra, removed []string) bool {
	lp := strings.ToLower(p)
	for _, r := range removed {
		if strings.HasPrefix(lp, strings.ToLower(r)) {
			return false
		}
	}
	for _, h := range extra {
		if strings.HasPrefix(lp, strings.ToLower(h)) {
			return true
		}
	}
	for _, h := range defaultHoneypots {
		if strings.Contains(lp, h) {
			return true
		}
	}
	if strings.HasPrefix(lp, "/.well-known/") &&
		!strings.HasPrefix(lp, "/.well-known/acme-challenge/") &&
		!strings.HasPrefix(lp, "/.well-known/openid-configuration") &&
		!strings.HasPrefix(lp, "/.well-known/change-password") {
		return true
	}
	if reTraversal.MatchString(lp) || reNullByte.MatchString(lp) {
		return true
	}
	return false
}

// classifyUA returns (class, isBrowserClaim, isSearchBotClaim, isAI).
func classifyUA(ua string) (class string, browser, search, ai bool) {
	l := strings.ToLower(ua)
	for _, t := range aiTokens {
		if strings.Contains(l, t) {
			return "ai-crawler", false, false, true
		}
	}
	for _, t := range scannerTokens {
		if strings.Contains(l, t) {
			return "scanner", false, false, false
		}
	}
	for _, t := range []string{"googlebot", "bingbot", "duckduckbot", "yandex", "baiduspider", "applebot"} {
		if strings.Contains(l, t) {
			return "search", false, true, false
		}
	}
	for _, t := range []string{"censys", "shodan", "internet-measurement", "binaryedge", "expanse", "stretchoid", "leakix"} {
		if strings.Contains(l, t) {
			return "research-scanner", false, false, false
		}
	}
	if strings.Contains(l, "mozilla/") && strings.Contains(l, "chrome") ||
		strings.Contains(l, "firefox/") {
		return "browser", true, false, false
	}
	if ua == "" {
		return "empty", false, false, false
	}
	if l == "mozilla/4.0" || l == "mozilla/5.0" || len(ua) < 8 {
		return "fake-ua", false, false, false
	}
	return "", false, false, false
}

var aiTokens, scannerTokens []string

func init() {
	aiTokens = loadList(aiCrawlersRaw)
	scannerTokens = loadList(scannersRaw)
}

// browserLikeMissingHeaders is a weak consistency check for navigation GETs.
func browserLikeMissingHeaders(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return false
	}
	if r.Header.Get("Accept-Language") == "" {
		return true
	}
	if r.Header.Get("Sec-Fetch-Mode") == "" && r.Header.Get("Sec-CH-UA") == "" {
		ua := r.UserAgent()
		if strings.Contains(ua, "Chrome/1") {
			return true
		}
	}
	return false
}

// isIPLiteralHost reports whether the Host header is a bare IP literal.
func isIPLiteralHost(host string) bool {
	host = strings.TrimSuffix(host, "]")
	h := strings.TrimPrefix(strings.SplitN(host, ":", 2)[0], "[")
	_, err := netip.ParseAddr(h)
	return err == nil
}
