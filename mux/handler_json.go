package mux

import "net/http"

func defaultJSONErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// HandleJSON returns an [http.Handler] that decodes the request body as JSON
// into a value of type In, calls fn, and encodes the returned Out value as a
// JSON response with status 200.
//
// If the JSON decoding fails or fn returns a non-nil error, onError is called
// instead of writing a success response. The caller controls how errors are
// mapped to HTTP status codes and response bodies. If onError is nil, a
// default handler writes the error message with status 500.
//
// The handler signature includes [http.ResponseWriter] and [*http.Request] so
// that fn can access route variables, headers, and other request metadata.
//
// Use [HandleJSONResponse] for handlers that do not consume a request body
// (e.g. GET or DELETE endpoints).
func HandleJSON[In, Out any](
	fn func(http.ResponseWriter, *http.Request, In) (Out, error),
	onError func(http.ResponseWriter, *http.Request, error),
) http.Handler {
	if onError == nil {
		onError = defaultJSONErrorHandler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in In
		if err := BindJSON(r, &in); err != nil {
			onError(w, r, err)
			return
		}

		out, err := fn(w, r, in)
		if err != nil {
			onError(w, r, err)
			return
		}

		ResponseJSON(w, http.StatusOK, out)
	})
}

// HandleJSONResponse returns an [http.Handler] that calls fn and encodes the
// returned Out value as a JSON response with status 200. Unlike [HandleJSON],
// it does not read or decode the request body, making it suitable for GET,
// DELETE, or other methods that do not carry a JSON payload.
//
// If fn returns a non-nil error, onError is called instead of writing a
// success response. The caller controls how errors are mapped to HTTP status
// codes and response bodies. If onError is nil, a default handler writes the
// error message with status 500.
func HandleJSONResponse[Out any](
	fn func(http.ResponseWriter, *http.Request) (Out, error),
	onError func(http.ResponseWriter, *http.Request, error),
) http.Handler {
	if onError == nil {
		onError = defaultJSONErrorHandler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := fn(w, r)
		if err != nil {
			onError(w, r, err)
			return
		}

		ResponseJSON(w, http.StatusOK, out)
	})
}
