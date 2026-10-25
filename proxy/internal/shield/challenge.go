package shield

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const passCookie = "nb-shield"
const verifyPath = "/.netbirdy-shield/verify"

// challengePayload binds the challenge to ip, ua, host, expiry.
func (s *Shield) challengePayload(ip netip.Addr, ua, host string, exp time.Time) string {
	msg := ip.String() + "|" + ua + "|" + host + "|" + strconv.FormatInt(exp.Unix(), 10)
	return msg
}

func (s *Shield) sign(msg string) string {
	m := hmac.New(sha256.New, s.hmacKey)
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

// serveChallenge serves the PoW interstitial; returns true when it handled.
func (s *Shield) serveChallenge(w http.ResponseWriter, r *http.Request, ip netip.Addr) bool {
	ua := r.UserAgent()
	host := strings.SplitN(r.Host, ":", 2)[0]
	if strings.HasPrefix(r.URL.Path, verifyPath) {
		s.handleVerify(w, r, ip)
		return true
	}
	exp := time.Now().Add(s.cfg.ChallengeTTL)
	payload := s.challengePayload(ip, ua, host, exp)
	sig := s.sign(payload)
	bits := s.cfg.ChallengeDifficulty
	nonce := fmt.Sprintf("%x", sha256.Sum256([]byte(payload+sig)))
	diff := "0000" // hex prefix target derived from bits
	// bits -> hex digits of leading zeros
	zeros := bits / 4
	if zeros < 1 {
		zeros = 1
	}
	if zeros > 8 {
		zeros = 8
	}
	diff = strings.Repeat("0", zeros)

	page := challengePage(nonce, diff, r.URL.RequestURI(), r.URL.Query().Get("exp"), host)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(page))
	return true
}

func (s *Shield) handleVerify(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	ua := r.UserAgent()
	host := strings.SplitN(r.Host, ":", 2)[0]
	expStr := r.URL.Query().Get("exp")
	nonce := r.URL.Query().Get("nonce")
	redir := r.URL.Query().Get("r")
	if redir == "" || !strings.HasPrefix(redir, "/") {
		redir = "/"
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		http.Error(w, "expired", http.StatusForbidden)
		return
	}
	payload := s.challengePayload(ip, ua, host, time.Unix(exp, 0))
	sig := s.sign(payload)
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(payload+sig)))
	if nonce != want {
		http.Error(w, "bad challenge", http.StatusForbidden)
		return
	}
	// verify proof: sha256(nonce:counter) has the required leading zeros
	counter := r.URL.Query().Get("n")
	diffBits := s.cfg.ChallengeDifficulty
	solved := sha256.Sum256([]byte(nonce + ":" + counter))
	zeros := diffBits / 4
	if zeros < 1 {
		zeros = 1
	}
	ok := strings.HasPrefix(hex.EncodeToString(solved[:]), strings.Repeat("0", min(zeros, 8)))
	if !ok {
		http.Error(w, "bad proof", http.StatusForbidden)
		return
	}
	expC := time.Now().Add(s.cfg.PassCookieTTL)
	mac := s.sign(ip.String() + "|" + ua + "|" + host + "|" + strconv.FormatInt(expC.Unix(), 10) + "|pass")
	val := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(expC.Unix(), 10) + "." + mac))
	c := &http.Cookie{Name: passCookie, Value: val, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, Expires: expC}
	http.SetCookie(w, c)
	http.Redirect(w, r, redir, http.StatusFound)
}

func (s *Shield) hasValidPass(r *http.Request, ip netip.Addr) bool {
	c, err := r.Cookie(passCookie)
	if err != nil {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(b), ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	host := strings.SplitN(r.Host, ":", 2)[0]
	want := s.sign(ip.String() + "|" + r.UserAgent() + "|" + host + "|" + parts[0] + "|pass")
	return hmac.Equal([]byte(parts[1]), []byte(want))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func challengePage(nonce, diff, back, exp, host string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>Checking</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>body{background:#000;color:#e8e8e8;background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='360' height='360'%3E%3Cg stroke='%23fff' stroke-opacity='0.06' fill='%23fff' fill-opacity='0.10'%3E%3Cpath d='M40 60 130 40M130 40 210 90M210 90 300 55M40 60 70 150M70 150 160 190M160 190 250 160M250 160 300 55M160 190 180 290M180 290 290 300M290 300 250 160M70 150 15 220M180 290 90 330M300 55 330 140'/%3E%3Ccircle cx='40' cy='60' r='2'/%3E%3Ccircle cx='130' cy='40' r='1.5'/%3E%3Ccircle cx='210' cy='90' r='2.2'/%3E%3Ccircle cx='300' cy='55' r='1.8'/%3E%3Ccircle cx='70' cy='150' r='2'/%3E%3Ccircle cx='160' cy='190' r='1.6'/%3E%3Ccircle cx='250' cy='160' r='2.4'/%3E%3Ccircle cx='15' cy='220' r='1.5'/%3E%3Ccircle cx='180' cy='290' r='2'/%3E%3Ccircle cx='290' cy='300' r='1.7'/%3E%3Ccircle cx='90' cy='330' r='1.5'/%3E%3Ccircle cx='330' cy='140' r='1.9'/%3E%3C/g%3E%3C/svg%3E");font-family:system-ui;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}div{text-align:center}p{color:#8a8a8a}</style></head>
<body><div><h2>Checking your browser</h2><p id="m">Working...</p><noscript><p>JavaScript is required.</p></noscript></div>
<script>
const nonce="` + nonce + `";const diff="` + diff + `";const back=` + strconv.Quote(back) + `;const exp=` + strconv.Quote(exp) + `;
async function go(){
  let i=0;
  const enc=new TextEncoder();
  for(;;i++){
    const h=await crypto.subtle.digest("SHA-256",enc.encode(nonce+":"+i));
    const hex=[...new Uint8Array(h)].map(b=>b.toString(16).padStart(2,"0")).join("");
    if(hex.startsWith(diff)){location=` + "`/.netbirdy-shield/verify?nonce=`" + `+nonce+` + "\"&n=\"" + `+i+` + "\"&r=\"" + `+encodeURIComponent(back)+` + "\"&exp=\"" + `+encodeURIComponent(exp||"");return}
    if(i>1e7){document.getElementById("m").textContent="Challenge too hard.";return}
  }
}
go();</script></body></html>`
}
