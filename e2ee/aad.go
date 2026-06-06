package e2ee

// AAD prefixes per the draft. Each directional AAD begins with the protocol
// version tag followed by a single space and the canonical E2EE-Session
// field(s).
const (
	aadReqPrefix = "e2ee/v1:req "
	aadResPrefix = "e2ee/v1:res "
)

// requestAAD builds the request AAD per the draft:
//
//	"e2ee/v1:req" || " " || <canonical E2EE-Session field>
func requestAAD(reqField string) []byte {
	out := make([]byte, 0, len(aadReqPrefix)+len(reqField))
	out = append(out, aadReqPrefix...)
	out = append(out, reqField...)

	return out
}

// responseAAD builds the response AAD per the draft:
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
