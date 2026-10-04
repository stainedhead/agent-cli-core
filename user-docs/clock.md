# clock

`clock` is a small public package for time you can control in tests. It was internal in v0.1.0 and is public in v0.2.0 (unreleased; tag pending), so your CLI can implement or reuse it.

```go
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}
```

- `clock.System{}` is the real clock. `Sleep` returns early with the context's error if the context is done.
- `clock.NewFake(start)` returns a deterministic `*clock.Fake`. Time moves only when you call `Advance(d)`, or instantly on every `Sleep` after `SetAutoAdvance(true)`. `Slept()` lists the durations requested in auto-advance mode, `Waiters()` counts blocked sleepers, and `BlockUntil(n)` waits until `n` goroutines are asleep so a test does not race `Advance`.

Where it plugs in:

| Place | Type |
|---|---|
| `audit.WithClock(c)` | `clock.Clock` (a nil clock keeps the system clock) |
| `httpx.Config.Clock` | `httpx.Clock`, the same two methods; any `clock.Clock` satisfies it |
| `policy.NewEngine(p, c)` | `policy.Clock`, which needs only `Now()`; any `clock.Clock` satisfies it |

```go
fc := clock.NewFake(time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
fc.SetAutoAdvance(true) // retries "sleep" instantly
client := httpx.NewClient(httpx.Config{Clock: fc})
logger, _ := audit.Open(audit.Config{Path: path}, audit.WithClock(fc))
```
