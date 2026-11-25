package handlers

import (
	"net/http"
	"sort"
	"strings"
)

// API is the explicit router.
//
// It is deliberately not http.DefaultServeMux: the route table is the HTTP
// contract for this server, so 404 and 405 have to be answered by our own
// envelope helper rather than by the mux's plain-text defaults.
type API struct {
	routes map[string]route
}

type route struct {
	methods map[string]http.HandlerFunc
}

// New returns the API with every route registered. Registration happens here
// and nowhere else, rather than through package-level init or global mux state.
func New() *API {
	a := &API{routes: make(map[string]route)}
	a.handle(http.MethodGet, "/", a.handleRoot)
	a.handle(http.MethodGet, "/health", a.handleHealth)
	a.handle(http.MethodGet, "/echo", a.handleEchoGet)
	a.handle(http.MethodPost, "/echo", a.handleEchoPost)
	return a
}

func (a *API) handle(method, path string, h http.HandlerFunc) {
	r, ok := a.routes[path]
	if !ok {
		r = route{methods: make(map[string]http.HandlerFunc)}
	}
	r.methods[method] = h
	a.routes[path] = r
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	routes, ok := a.routes[r.URL.Path]
	if !ok {
		WriteError(w, http.StatusNotFound, "not_found", "no route matches this path")
		return
	}

	h, ok := routes.methods[r.Method]
	if !ok {
		if allow := routes.allow(); allow != "" {
			w.Header().Set("Allow", allow)
		}
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this path")
		return
	}

	h(w, r)
}

// allow renders the Allow header value, sorted so the response is deterministic.
func (r route) allow() string {
	methods := make([]string, 0, len(r.methods))
	for m := range r.methods {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	return strings.Join(methods, ", ")
}
