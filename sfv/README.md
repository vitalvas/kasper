# sfv

Structured Field Values for HTTP (RFC 9651) for Go. RFC 9651 obsoletes
RFC 8941 and adds the Date and Display String bare-item types.

## Types

Structured Fields are built from three top-level types, each of which may
carry parameters.

| Type | Go type | Description |
|------|---------|-------------|
| Item | `Item` | A bare item with optional parameters |
| List | `List` | A sequence of members, each an Item or Inner List |
| Dictionary | `Dictionary` | An ordered mapping of keys to members |

Bare items:

| Kind | Constructor | Example wire form |
|------|-------------|-------------------|
| Integer | `Integer(int64)` | `42` |
| Decimal | `Decimal(float64)` | `3.14` |
| String | `String(string)` | `"hello"` |
| Token | `Token(string)` | `application/json` |
| Byte Sequence | `ByteSequence([]byte)` | `:aGVsbG8=:` |
| Boolean | `Boolean(bool)` | `?1` |
| Date | `Date(time.Time)` | `@1659578233` |
| Display String | `DisplayString(string)` | `%"caf%c3%a9"` |

## Parsing

Each top-level type has a parser. Parsing is strict: any characters remaining
after a complete value produce `ErrTrailing`, and malformed input produces an
error wrapping `ErrParse`.

```go
// Item with parameters.
it, err := sfv.ParseItem(`10;a=1;b="x";c`)
if err != nil {
    log.Fatal(err)
}
fmt.Println(it.Value.Integer) // 10
a, ok := it.Params.Get("a")   // a.Integer == 1, ok == true

// List with inner lists.
l, err := sfv.ParseList(`("foo" "bar");lvl=5, ("baz")`)

// Dictionary.
d, err := sfv.ParseDictionary(`en="Applepie", da=:aGVsbG8=:`)
en, ok := d.Get("en")         // en.Item.Value.Str == "Applepie"
```

## Serializing

Each value type has a `String` method producing the canonical serialization
defined by RFC 9651 Section 4.

```go
it := sfv.Item{
    Value: sfv.Integer(5),
    Params: sfv.Parameters{
        {Key: "a", Value: sfv.Integer(1)},
        {Key: "b", Value: sfv.Boolean(true)},
    },
}
fmt.Println(it.String()) // 5;a=1;b

l := sfv.List{
    {Item: sfv.Item{Value: sfv.Token("a")}},
    {Item: sfv.Item{Value: sfv.String("b")}},
}
fmt.Println(l.String()) // a, "b"
```

## Semantics

- Duplicate keys in a Dictionary or Parameters follow last-wins semantics while
  keeping the key's earliest position.
- Boolean-true members serialize as the bare key (`a` rather than `a=?1`).
- Decimals serialize with at least one and at most three fractional digits,
  rounded half-to-even.
- Display Strings hold Unicode; non-ASCII bytes are percent-encoded as UTF-8.

## Errors

| Error | Description |
|-------|-------------|
| `ErrParse` | Input does not conform to the RFC 9651 grammar |
| `ErrTrailing` | Characters remain after a complete value |

## Standards

- [RFC 9651](https://www.rfc-editor.org/rfc/rfc9651) - Structured Field Values for HTTP
- [RFC 8941](https://www.rfc-editor.org/rfc/rfc8941) - Structured Field Values for HTTP (obsoleted by RFC 9651)
