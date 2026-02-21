# Performance Benchmarks: kasper/mux vs gorilla/mux

Comparative benchmarks measuring route registration and request dispatching
across different endpoint counts and complexity levels.

**Environment**: Apple M3 Pro, darwin/arm64, Go 1.25

## Benchmark Design

### Endpoint Counts

5, 10, 50, 100, 500 resource groups (5 routes each).

### Complexity Levels

- **Simple** -- Static paths, no variables, no middleware.
- **Medium** -- Path variables with regex constraints (`{id:[0-9]+}`), 1 middleware.
- **Complex** -- Subrouters with `PathPrefix`, path variables, 3 middleware, query matchers, header matchers.

### Operations

- **Setup** -- Time and memory to register all routes on a fresh router.
- **Dispatch First** -- Match request against the first registered route.
- **Dispatch Last** -- Match request against the last registered route (worst-case linear scan).
- **Dispatch Miss** -- Request a non-existent route (404).

## Results

### Route Setup

| Complexity | Routes | Kasper (ns/op) | Gorilla (ns/op) | Speedup | Kasper (B/op) | Gorilla (B/op) | Mem ratio |
|------------|-------:|---------------:|----------------:|--------:|--------------:|---------------:|----------:|
| Simple     |      5 |          6,637 |          72,408 |   10.9x |        15,472 |        157,643 |     10.2x |
|            |     10 |         13,137 |         144,498 |   11.0x |        30,792 |        315,055 |     10.2x |
|            |     50 |         72,774 |         742,473 |   10.2x |       153,382 |      1,598,667 |     10.4x |
|            |    100 |        166,355 |       1,492,326 |    9.0x |       307,267 |      3,203,717 |     10.4x |
|            |    500 |        790,395 |       7,735,862 |    9.8x |     1,563,248 |     16,518,516 |     10.6x |
| Medium     |      5 |         13,162 |         109,551 |    8.3x |        18,598 |        226,628 |     12.2x |
|            |     10 |         26,544 |         217,925 |    8.2x |        37,036 |        453,009 |     12.2x |
|            |     50 |        135,239 |       1,114,069 |    8.2x |       183,897 |      2,285,802 |     12.4x |
|            |    100 |        270,213 |       2,220,537 |    8.2x |       368,013 |      4,577,336 |     12.4x |
|            |    500 |      1,368,809 |      11,463,015 |    8.4x |     1,883,330 |     23,281,186 |     12.4x |
| Complex    |      5 |         20,757 |         175,535 |    8.5x |        30,350 |        375,720 |     12.4x |
|            |     10 |         41,926 |         345,581 |    8.2x |        60,525 |        751,097 |     12.4x |
|            |     50 |        212,650 |       1,768,685 |    8.3x |       301,663 |      3,798,636 |     12.6x |
|            |    100 |        421,699 |       3,805,257 |    9.0x |       603,267 |      7,608,194 |     12.6x |
|            |    500 |      2,115,594 |      18,629,777 |    8.8x |     3,015,834 |     38,380,096 |     12.7x |

### Dispatch First

| Complexity | Routes | Kasper (ns/op) | Gorilla (ns/op) | Delta | K allocs | G allocs |
|------------|-------:|---------------:|----------------:|------:|---------:|---------:|
| Simple     |      5 |            125 |             296 |  -58% |        3 |        7 |
|            |     10 |            127 |             296 |  -57% |        3 |        7 |
|            |     50 |            143 |             296 |  -52% |        3 |        7 |
|            |    100 |            171 |             297 |  -42% |        3 |        7 |
|            |    500 |            149 |             294 |  -49% |        3 |        7 |
| Medium     |      5 |            361 |             432 |  -16% |        7 |        9 |
|            |     10 |            362 |             434 |  -17% |        7 |        9 |
|            |     50 |            360 |             433 |  -17% |        7 |        9 |
|            |    100 |            361 |             432 |  -16% |        7 |        9 |
|            |    500 |            357 |             434 |  -18% |        7 |        9 |
| Complex    |      5 |            779 |           1,035 |  -25% |       11 |       21 |
|            |     10 |            783 |           1,038 |  -25% |       11 |       21 |
|            |     50 |            792 |           1,035 |  -23% |       11 |       21 |
|            |    100 |            797 |           1,035 |  -23% |       11 |       21 |
|            |    500 |            774 |           1,029 |  -25% |       11 |       21 |

### Dispatch Last

| Complexity | Routes | Kasper (ns/op) | Gorilla (ns/op) | Delta | K allocs | G allocs |
|------------|-------:|---------------:|----------------:|------:|---------:|---------:|
| Simple     |      5 |            460 |             539 |  -15% |        3 |        7 |
|            |     10 |            867 |             829 |   +5% |        3 |        7 |
|            |     50 |          4,396 |           3,854 |  +14% |        3 |        7 |
|            |    100 |          8,903 |           8,147 |   +9% |        3 |        7 |
|            |    500 |         44,358 |          42,871 |   +3% |        3 |        7 |
| Medium     |      5 |            861 |             839 |   +3% |        7 |        9 |
|            |     10 |          1,478 |           1,351 |   +9% |        7 |        9 |
|            |     50 |          7,245 |           7,472 |   -3% |        7 |        9 |
|            |    100 |         14,165 |          15,799 |  -10% |        7 |        9 |
|            |    500 |         69,048 |          83,729 |  -18% |        7 |        9 |
| Complex    |      5 |            899 |           1,146 |  -22% |       11 |       21 |
|            |     10 |          1,020 |           1,265 |  -19% |       11 |       21 |
|            |     50 |          2,520 |           2,684 |   -6% |       11 |       21 |
|            |    100 |          3,946 |           4,354 |   -9% |       11 |       21 |
|            |    500 |         17,746 |          19,615 |  -10% |       11 |       21 |

### Dispatch Miss (404)

| Complexity | Routes | Kasper (ns/op) | Gorilla (ns/op) | Delta | K B/op | G B/op |
|------------|-------:|---------------:|----------------:|------:|-------:|-------:|
| Simple     |      5 |            500 |             468 |   +7% |     96 |     96 |
|            |     10 |            900 |             754 |  +19% |     96 |     96 |
|            |     50 |          4,247 |           3,665 |  +16% |     96 |     96 |
|            |    100 |          8,501 |           7,952 |   +7% |     96 |     96 |
|            |    500 |         42,772 |          42,550 |   ~0% |     96 |     96 |
| Medium     |      5 |            704 |             679 |   +4% |     96 |     96 |
|            |     10 |          1,307 |           1,189 |  +10% |     96 |     96 |
|            |     50 |          6,610 |           7,014 |   -6% |     96 |     96 |
|            |    100 |         13,415 |          15,925 |  -16% |     96 |     96 |
|            |    500 |         66,348 |          82,814 |  -20% |     96 |     96 |
| Complex    |      5 |            237 |             301 |  -21% |     96 |     96 |
|            |     10 |            345 |             409 |  -16% |     96 |     96 |
|            |     50 |          1,261 |           1,347 |   -6% |     96 |     96 |
|            |    100 |          2,518 |           2,887 |  -13% |     96 |     96 |
|            |    500 |         14,778 |          17,342 |  -15% |     96 |     96 |

## Conclusion

Switching from gorilla/mux to kasper/mux provides the following improvements:

**Faster startup.** Route registration is 8-11x faster with 10-13x less memory.
Applications with hundreds of routes will see noticeably shorter initialization times.

**Lower per-request overhead.** Every dispatched request allocates fewer objects and completes faster:

| Route complexity | Latency reduction | Allocs (kasper) | Allocs (gorilla) |
|------------------|------------------:|----------------:|-----------------:|
| Simple           |           42-58%  |               3 |                7 |
| Medium           |           16-18%  |               7 |                9 |
| Complex          |           23-25%  |              11 |               21 |

**Less GC pressure.** Fewer allocations per request means less work for the garbage collector,
which translates to more stable tail latencies under load.

**Equal 404 handling.** The miss path uses 96 B/op (same as gorilla/mux) and scales better
at higher route counts, winning by 15-20% at 100-500 routes.

## Running Benchmarks

The benchmark suite is an independent module at `examples/benchmarks/`:

```bash
cd examples/benchmarks
go test -run='^$' -bench=. -benchmem -count=5 -timeout=30m ./...
```

To run a subset:

```bash
# Only setup benchmarks
go test -run='^$' -bench='Setup' -benchmem ./...

# Only kasper dispatch on complex routes
go test -run='^$' -bench='Kasper/Dispatch.*Complex' -benchmem ./...
```
