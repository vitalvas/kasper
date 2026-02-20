# Changelog

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
