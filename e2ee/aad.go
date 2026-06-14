package e2ee

// AAD construction per draft-vasylenko-e2ee-http Section 7.4 (Additional
// Authenticated Data). Each directional AAD begins with the protocol version
// tag followed by a single space and the canonical, deterministically
// re-serialized E2EE-Session field(s) (RFC 9651), binding kid, aead, epk, ts,
// and nid to the ciphertext.
const (
	aadReqPrefix = "e2ee/v1:req "
	aadResPrefix = "e2ee/v1:res "
)

// requestAAD builds the request AAD per draft-vasylenko-e2ee-http Section 7.4:
//
//	"e2ee/v1:req" || " " || <canonical E2EE-Session field>
func requestAAD(reqField string) []byte {
	out := make([]byte, 0, len(aadReqPrefix)+len(reqField))
	out = append(out, aadReqPrefix...)
	out = append(out, reqField...)

	return out
}

// responseAAD builds the response AAD per draft-vasylenko-e2ee-http
// Section 7.4. Binding both the request and response fields prevents response
// substitution:
//
//	"e2ee/v1:res" || " " || <request field> || " " || <response field>
func responseAAD(reqField, resField string) []byte {
	out := make([]byte, 0, len(aadResPrefix)+len(reqField)+1+len(resField))
	out = append(out, aadResPrefix...)
	out = append(out, reqField...)
	out = append(out, ' ')
	out = append(out, resField...)

	return out
}
