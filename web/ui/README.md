# web/ui

The proxmops web UI: Vue 3 + Vite + TailwindCSS v4 + shadcn-vue.

## Develop

```sh
bun install
bun run dev      # Vite dev server, proxies /api to the daemon on :8080
```

Run the daemon separately (`go run ./cmd/proxmops daemon`) so the UI has an API
to talk to.

## Build

```sh
bun run build    # outputs static assets to dist/
```

The Go binary embeds `dist/` via `go:embed` (see `web/embed.go`) and serves it,
with an SPA fallback so client-side routes resolve on a full page load. `make ui`
runs this build; `make build` runs it before compiling the binary.

## Theming

Design tokens live in `src/style.css` as shadcn CSS variables (oklch). The block
is compatible with [tweakcn](https://tweakcn.com): paste an exported theme to
restyle the whole UI.

## Note

Built assets (`dist/`) and `node_modules/` are not committed; they are produced
by the build. Only `dist/.gitkeep` is tracked so `go:embed` always compiles.
