package main

import (
	"net/http"
	"net/url"
	"strings"
)

// sameOriginRuntime refuses runtime calls that come from any page but our
// own. Wails answers every request to http://wails.localhost/wails/…, and a
// remote page shown in one of our windows (GGG's sign-in page) could send one
// with a plain cross-origin POST: the browser would not let it read the
// answer, but the bound method would still run. Such a request always carries
// an Origin header naming the remote site, so it is turned away here, before
// Wails' own handlers see it.
func sameOriginRuntime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/wails/") && !trustedOrigin(req.Header.Get("Origin")) {
			http.Error(rw, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(rw, req)
	})
}

// trustedOrigin accepts our own pages (served from wails.localhost) and
// requests without an Origin, which browsers send only for same-origin GETs
// such as loading runtime.js.
func trustedOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "wails.localhost" || strings.HasSuffix(host, ".wails.localhost")
}
