package mux

// Common MIME content type constants.
//
// See: https://www.iana.org/assignments/media-types/media-types.xhtml
const (
	// Application types.
	ContentTypeApplicationJSON           = "application/json"
	ContentTypeApplicationProblemJSON    = "application/problem+json"
	ContentTypeApplicationXML            = "application/xml"
	ContentTypeApplicationFormURLEncoded = "application/x-www-form-urlencoded"
	ContentTypeApplicationOctetStream    = "application/octet-stream"
	ContentTypeApplicationPDF            = "application/pdf"
	ContentTypeApplicationZip            = "application/zip"
	ContentTypeApplicationGzip           = "application/gzip"
	ContentTypeApplicationJavaScript     = "application/javascript"
	ContentTypeApplicationYAML           = "application/x-yaml"
	ContentTypeApplicationXGzip          = "application/x-gzip"
	ContentTypeApplicationXBzip2         = "application/x-bzip2"
	ContentTypeApplicationXXZ            = "application/x-xz"
	ContentTypeApplicationZstd           = "application/zstd"
	ContentTypeApplicationX7ZCompressed  = "application/x-7z-compressed"
	ContentTypeApplicationXRARCompressed = "application/x-rar-compressed"

	// Multipart types.
	ContentTypeMultipartFormData = "multipart/form-data"

	// Text types.
	ContentTypeTextPlain       = "text/plain"
	ContentTypeTextHTML        = "text/html"
	ContentTypeTextCSS         = "text/css"
	ContentTypeTextCSV         = "text/csv"
	ContentTypeTextXML         = "text/xml"
	ContentTypeTextMarkdown    = "text/markdown"
	ContentTypeTextEventStream = "text/event-stream"

	// Image types.
	ContentTypeImagePNG    = "image/png"
	ContentTypeImageJPEG   = "image/jpeg"
	ContentTypeImageGIF    = "image/gif"
	ContentTypeImageWebP   = "image/webp"
	ContentTypeImageSVGXML = "image/svg+xml"
)
