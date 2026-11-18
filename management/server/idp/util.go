package idp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/server/util"
)

var (
	lowerCharSet   = "abcdedfghijklmnopqrst"
	upperCharSet   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	specialCharSet = "!@#$%&*"
	numberSet      = "0123456789"
	allCharSet     = lowerCharSet + upperCharSet + specialCharSet + numberSet
)

type JsonParser struct{}

func (JsonParser) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (JsonParser) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// GeneratePassword generates user password
func GeneratePassword(passwordLength, minSpecialChar, minNum, minUpperCase int) string {
	var password strings.Builder

	//Set special character
	for i := 0; i < minSpecialChar; i++ {
		random := util.RandIntn(len(specialCharSet))
		password.WriteString(string(specialCharSet[random]))
	}

	//Set numeric
	for i := 0; i < minNum; i++ {
		random := util.RandIntn(len(numberSet))
		password.WriteString(string(numberSet[random]))
	}

	//Set uppercase
	for i := 0; i < minUpperCase; i++ {
		random := util.RandIntn(len(upperCharSet))
		password.WriteString(string(upperCharSet[random]))
	}

	remainingLength := passwordLength - minSpecialChar - minNum - minUpperCase
	for i := 0; i < remainingLength; i++ {
		random := util.RandIntn(len(allCharSet))
		password.WriteString(string(allCharSet[random]))
	}
	inRune := []rune(password.String())
	for i := len(inRune) - 1; i > 0; i-- {
		j := util.RandIntn(i + 1)
		inRune[i], inRune[j] = inRune[j], inRune[i]
	}
	return string(inRune)
}

// baseURL returns the base url  by concatenating
// the scheme and host components of the parsed URL.
func baseURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	return parsedURL.Scheme + "://" + parsedURL.Host
}

const (
	// Provides the env variable name for use with idpTimeout function
	idpTimeoutEnv = "NB_IDP_TIMEOUT"
	// Sets the defaultTimeout to 10s.
	defaultTimeout = 10 * time.Second
)

// errRedirectRefused is returned instead of http.ErrUseLastResponse so the
// client closes the redirect response rather than handing it back unread.
var errRedirectRefused = errors.New("redirect refused")

func newHTTPClient() *http.Client {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.MaxIdleConns = 5

	return &http.Client{
		Timeout:   idpTimeout(),
		Transport: httpTransport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirectRefused
		},
	}
}

// idpTimeout returns a timeout value for the IDP
func idpTimeout() time.Duration {
	timeoutStr, ok := os.LookupEnv(idpTimeoutEnv)
	if !ok || timeoutStr == "" {
		return defaultTimeout
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return defaultTimeout
	}
	return timeout
}

// Body bounds for IdP responses. A malfunctioning or hostile endpoint
// otherwise buffers arbitrary payload into memory per request.
const (
	maxIdPResponseBytes     = 32 << 20
	maxExportPlaintextBytes = 2 << 30
	errBodyLimitExceeded    = "response body exceeds limit"
)

// readBodyLimit reads r fully, failing when more than limit bytes are present.
func readBodyLimit(r io.Reader, limit int64) ([]byte, error) {
	lr := io.LimitReader(r, limit+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New(errBodyLimitExceeded)
	}
	return b, nil
}
