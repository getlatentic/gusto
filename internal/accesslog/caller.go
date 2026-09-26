package accesslog

import (
	"net/http"
	"strings"
)

// caller is how the request tried to identify itself: "bearer" for the shared
// password in an Authorization header, "cookie" for a sign-in cookie, "none"
// otherwise. It is only ever the kind. Whether the credential held is the status.
//
// Bearer comes first because the API reads only the header, whatever cookie a
// program happens to carry. Any cookie counts as a cookie, because the sign-in
// is the only one this server sets, and naming it here would tie this package to
// the pages it wraps.
func caller(r *http.Request) string {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return "bearer"
	}
	if len(r.Cookies()) > 0 {
		return "cookie"
	}
	return "none"
}
