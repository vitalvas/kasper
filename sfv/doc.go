// Package sfv implements Structured Field Values for HTTP per RFC 9651,
// which obsoletes RFC 8941 and adds the Date and Display String bare-item
// types.
//
// Structured Fields give HTTP field values a common, unambiguous typed
// syntax built from three top-level types:
//
//   - Item: a bare item with optional parameters.
//   - List: a sequence of members, each an Item or an Inner List, with
//     optional parameters.
//   - Dictionary: an ordered mapping of keys to Items or Inner Lists, with
//     optional parameters.
//
// Bare items are Integer, Decimal, String, Token, Byte Sequence, Boolean,
// Date, and Display String.
//
// Parse with ParseItem, ParseList, or ParseDictionary. Each value type has a
// String method producing the canonical serialization defined by RFC 9651
// Section 4.
package sfv
