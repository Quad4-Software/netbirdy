package middleware

import "net/http"

// maxAPIRequestBody bounds management API request bodies. Handlers decode
// JSON straight off r.Body, so an unbounded body lets any client, including
// unauthenticated requests on bypassed paths, pin memory proportional to the
// payload size.
const maxAPIRequestBody = 8 << 20

// bodyLimit wraps the request body in an http.MaxBytesReader so handlers see
// a read error past the cap instead of buffering arbitrarily large payloads.
func bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequestBody)
		}
		next.ServeHTTP(w, r)
	})
}
