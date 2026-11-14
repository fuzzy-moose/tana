# Follow-ups

- Optionally make useCollectorResource return a stable refresh/update with useCallback, so callers that subscribe to refresh stop re-subscribing on every render (findings 1/12).
- Optionally pass the transaction ctx to the favorites category selector, func(context.Context, *dbgen.Queries) ([]int, error) (findings 4/6).
- Check the sidenav quick action's compact layout at 640px and below by eye, as the spec's manual checks list it (finding 18).
