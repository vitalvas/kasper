package mux

import (
	"net/http"
	"strings"
)

// CORSMethodMiddleware automatically sets the Access-Control-Allow-Methods
// response header (Fetch Standard, CORS protocol) on requests to allow all
// methods that are registered for the route that matches the request.
func CORSMethodMiddleware(r *Router) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			allMethods, err := getAllMethodsForRoute(r, req)
			if err == nil {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(allMethods, ","))
			}
			next.ServeHTTP(w, req)
		})
	}
}

// getAllMethodsForRoute returns all HTTP methods registered for routes
// matching the given request's path.
func getAllMethodsForRoute(router *Router, req *http.Request) ([]string, error) {
	var allMethods []string

	for _, route := range router.routes {
		methods, err := route.GetMethods()
		if err != nil {
			continue
		}

		// Try matching with each of the route's methods.
		for _, method := range methods {
			testReq := req.Clone(req.Context())
			testReq.Method = method
			match := &RouteMatch{}
			if route.Match(testReq, match) {
				allMethods = append(allMethods, method)
			}
		}
	}

	if len(allMethods) == 0 {
		return nil, ErrNotFound
	}

	return allMethods, nil
}
