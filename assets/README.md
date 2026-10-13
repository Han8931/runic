# Runic assets

- `runic_demo.gif` — the animated demo shown in the root [README](../README.md).

Re-record it with:

```bash
tools/demo/make-demo.sh
```

That script builds `ru`, sets up a throwaway daemon and sample repo under
`/tmp/runic-demo`, replays [`tools/demo/demo.steps`](../tools/demo/demo.steps)
through a pty, and renders the result with
[agg](https://docs.asciinema.org/manual/agg/). Nothing it does touches your own
daemon, config, or repositories.
