# Changelog

## [0.3.0](https://github.com/vitalvas/kasper/compare/v0.2.0...v0.3.0) (2026-02-22)


### Features

* **openapi:** add SwaggerUIConfig and make HandleConfig optional ([08df124](https://github.com/vitalvas/kasper/commit/08df12415bfab3b0e6b67c8bd51cf2f3d11df7cd))

## [0.2.0](https://github.com/vitalvas/kasper/compare/v0.1.0...v0.2.0) (2026-02-22)


### Features

* **httpsig:** add HTTP Message Signatures (RFC 9421) package ([fe28e54](https://github.com/vitalvas/kasper/commit/fe28e54854eade52b8a0563cfedd654d683d0c55))
* **muxhandlers:** add method override and content-type check middleware ([404c804](https://github.com/vitalvas/kasper/commit/404c804978bda6591bcb418a36ec88077e1732ad))
* **muxhandlers:** add recovery, request ID, and request size limit middleware ([2713063](https://github.com/vitalvas/kasper/commit/27130635c3f9f5d80197e5ca9b48303eefc7d478))
* **muxhandlers:** add server and cache-control middleware ([d9886f5](https://github.com/vitalvas/kasper/commit/d9886f5c0aa85452375ac85c6acef0badbdc4ae2))
* **muxhandlers:** add static files handler with SPA fallback ([9a6c68f](https://github.com/vitalvas/kasper/commit/9a6c68ffbad4b72fb62eed3d8d7f1fca3c352a59))
* **muxhandlers:** add timeout, compression, and security headers middleware ([a4101a7](https://github.com/vitalvas/kasper/commit/a4101a7ef713c29f7facb16a2980c3f8c4be3e3c))


### Bug Fixes

* **muxhandlers:** pass request ID via context ([f394f39](https://github.com/vitalvas/kasper/commit/f394f39da043fd0d75613605a86cc848b451363d))
* **websocket:** enforce RFC 6455 masking, control frame RSV1, close payload, and NextWriter validation ([41dc0cd](https://github.com/vitalvas/kasper/commit/41dc0cde2ac9f790d4fd4b5bbd4fbea416af528b))

## [0.1.0](https://github.com/vitalvas/kasper/compare/v0.0.1...v0.1.0) (2026-02-20)


### Features

* add RFC 6455 compliant WebSocket implementation ([720f8c7](https://github.com/vitalvas/kasper/commit/720f8c75043a88db2dc71fcf20f3ba22d1a324f9))
* **mux:** add BindJSON and BindXML request binding helpers ([c43f1b3](https://github.com/vitalvas/kasper/commit/c43f1b35c44891381a8029144bd63269190c0942))
* **mux:** add domain macro with RFC 1035/1123 validation and varMatcher interface ([2eb4c60](https://github.com/vitalvas/kasper/commit/2eb4c60ee13ddb4f7f5a05b74a451777508f4b7b))
* **mux:** add drop-in replacement for gorilla/mux with optimized performance ([9039e58](https://github.com/vitalvas/kasper/commit/9039e58143324a2f017165c584fe87ce77cb1847))
* **mux:** add pattern macros and regexp cache for route variables ([e088d68](https://github.com/vitalvas/kasper/commit/e088d683ac8e31b144e54e0f3a22df13df35a609))
* **mux:** add ResponseJSON and ResponseXML response helpers ([0b4d258](https://github.com/vitalvas/kasper/commit/0b4d2582121271339ffea2860f81125d8c148dbf))
* **mux:** add subrouter NotFoundHandler, VarGet, and complete API docs ([00a8161](https://github.com/vitalvas/kasper/commit/00a8161286715df58e81eac7323f5d7679179219))
* **muxhandlers:** add Basic Auth middleware with RFC 7617 support ([0c7daea](https://github.com/vitalvas/kasper/commit/0c7daea259e2c05acf919bc92d7912f2f644289b))
* **muxhandlers:** add full CORS middleware with RFC-compliant protocol support ([dbe5de7](https://github.com/vitalvas/kasper/commit/dbe5de7a460954ceb6b7997ed9c9fe1ae53078e3))
* **muxhandlers:** add Proxy Headers middleware with trusted proxy validation ([31f1624](https://github.com/vitalvas/kasper/commit/31f162477be663682e48d6b74c34c4f005cc0cf9))
* **openapi:** add OpenAPI v3.1.0 specification generator ([e4bb315](https://github.com/vitalvas/kasper/commit/e4bb31537f69b48a95ae600377b0bcdbbb030cfb))
* **websocket:** add HTTP/2 support and unified HTTPClient configuration ([0450cdf](https://github.com/vitalvas/kasper/commit/0450cdf2258bc946177dc84000004559442eeac7))


### Bug Fixes

* **mux:** align API and behavior with gorilla/mux for drop-in compatibility ([fca356e](https://github.com/vitalvas/kasper/commit/fca356ef126efcd05c40c631db537c4fb3aebf5c))
* **mux:** fix RFC violations and add RFC reference comments ([8ba1013](https://github.com/vitalvas/kasper/commit/8ba101303f4292d1d799a907fb34d3e7c04efdb7))
* **openapi:** add spec references, fix schema collisions, YAML serialization, and operationId bugs ([662a763](https://github.com/vitalvas/kasper/commit/662a763222181a2c74a4141e079e60dc4c6d9cc5))
* **websocket:** add HTTP/2 WebSocket coverage for dialHTTP2 and upgradeHTTP2 ([4a6d7fd](https://github.com/vitalvas/kasper/commit/4a6d7fd3428736c8d73d7084c08b50d32ee0bfec))


### Performance Improvements

* **mux:** optimize dispatch hot path and add benchmarks ([a3487f3](https://github.com/vitalvas/kasper/commit/a3487f3ea72bf24441676a8bcf654105636874a8))
