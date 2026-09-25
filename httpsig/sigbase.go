package httpsig

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vitalvas/kasper/sfv"
)

// signatureParams holds the parameters that appear in the @signature-params
// component of the signature base.
type signatureParams struct {
	components []string
	created    time.Time
	expires    time.Time
	nonce      string
	alg        Algorithm
	keyID      string
	tag        string
}

// buildSignatureBase constructs the signature base string per RFC 9421
// Section 2.5. Each covered component produces a line
// "<component-id>": <value>\n and the final line is
// "@signature-params": <params>.
func buildSignatureBase(r *http.Request, params signatureParams) ([]byte, string, error) {
	var base strings.Builder

	for _, id := range params.components {
		val, err := componentValue(id, r)
		if err != nil {
			return nil, "", err
		}

		fmt.Fprintf(&base, "%q: %s\n", id, val)
	}

	sigParamsStr := serializeSignatureParams(params)
	fmt.Fprintf(&base, "\"@signature-params\": %s", sigParamsStr)

	return []byte(base.String()), sigParamsStr, nil
}

// serializeSignatureParams produces the inner-list representation of the
// signature parameters per RFC 9421 Section 2.3 and RFC 8941 Section 3.1.1.
//
// Format: (<component-ids>);<key>=<value>;...
func serializeSignatureParams(params signatureParams) string {
	var b strings.Builder

	// Inner list of component identifiers.
	b.WriteByte('(')
	for i, id := range params.components {
		if i > 0 {
			b.WriteByte(' ')
		}

		b.WriteString(strconv.Quote(id))
	}
	b.WriteByte(')')

	// Parameters.
	if !params.created.IsZero() {
		fmt.Fprintf(&b, ";created=%d", params.created.Unix())
	}

	if !params.expires.IsZero() {
		fmt.Fprintf(&b, ";expires=%d", params.expires.Unix())
	}

	if params.nonce != "" {
		b.WriteString(";nonce=")
		b.WriteString(quoteRFC8941(params.nonce))
	}

	b.WriteString(";alg=")
	b.WriteString(quoteRFC8941(params.alg.String()))
	b.WriteString(";keyid=")
	b.WriteString(quoteRFC8941(params.keyID))

	if params.tag != "" {
		b.WriteString(";tag=")
		b.WriteString(quoteRFC8941(params.tag))
	}

	return b.String()
}

// parseSignatureParams parses a signature parameters string as produced by
// serializeSignatureParams, using the sfv package (RFC 9651) as the underlying
// structured-field parser. It extracts the inner list of component identifiers
// and the key-value parameters into a signatureParams. The signature base is
// reconstructed by re-serializing these values, so parsing does not need to
// preserve the raw bytes.
//
// Expected format: ("@method" "@authority" "@path");created=...;keyid="..."
func parseSignatureParams(raw string) (signatureParams, error) {
	var params signatureParams

	// The @signature-params value is an sfv Inner List of component
	// identifiers carrying the signature parameters. It parses as a List with
	// a single inner-list member.
	list, err := sfv.ParseList(raw)
	if err != nil {
		return params, fmt.Errorf("%w: %v", ErrMalformedHeader, err)
	}
	if len(list) != 1 || !list[0].IsInnerList {
		return params, fmt.Errorf("%w: signature params must be an inner list", ErrMalformedHeader)
	}
	inner := list[0].InnerList

	for _, ci := range inner.Items {
		if ci.Value.Kind != sfv.KindString {
			return params, fmt.Errorf("%w: component id must be a string", ErrMalformedHeader)
		}
		params.components = append(params.components, ci.Value.Str)
	}

	for _, p := range inner.Params {
		switch p.Key {
		case "created":
			if p.Value.Kind != sfv.KindInteger {
				return params, fmt.Errorf("%w: invalid created timestamp", ErrMalformedHeader)
			}
			params.created = time.Unix(p.Value.Integer, 0)
		case "expires":
			if p.Value.Kind != sfv.KindInteger {
				return params, fmt.Errorf("%w: invalid expires timestamp", ErrMalformedHeader)
			}
			params.expires = time.Unix(p.Value.Integer, 0)
		case "nonce":
			params.nonce = p.Value.Str
		case "alg":
			params.alg = Algorithm(p.Value.Str)
		case "keyid":
			params.keyID = p.Value.Str
		case "tag":
			params.tag = p.Value.Str
		}
	}

	if params.alg == "" {
		return params, fmt.Errorf("%w: missing alg parameter", ErrMalformedHeader)
	}

	if params.keyID == "" {
		return params, fmt.Errorf("%w: missing keyid parameter", ErrMalformedHeader)
	}

	return params, nil
}

// quoteRFC8941 produces an RFC 8941 / RFC 9651 quoted-string. Only backslash
// and double-quote are escaped (Section 3.3.3). This is the byte authority for
// the signed @signature-params value, so it is kept in httpsig rather than
// delegated to sfv to guarantee the signature base is stable.
func quoteRFC8941(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' || ch == '"' {
			b.WriteByte('\\')
		}

		b.WriteByte(ch)
	}

	b.WriteByte('"')

	return b.String()
}
