`withdraw()` drains connections **before** it swaps the router entry, so a bad
build stays reachable for 31s worst case, against a 15 minute canary:

```go
func (r *Rollout) withdraw(ctx context.Context, build string) error {
    // The swap first, or the drain feeds the build being withdrawn.
    if err := r.router.point(ctx, r.previous); err != nil {
        return err
    }
    return r.pool.drain(ctx, build, 30*time.Second)
}
```

One thing needs your call before I touch it.
