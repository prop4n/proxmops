# web/ui

Reserved for the proxmops web UI.

The front-end stack is not chosen yet. Whatever is used, its production build
must output static files into `web/ui/dist/`, which the Go binary embeds through
`go:embed` (see `web/embed.go`) and serves from the daemon.

Until then, `dist/index.html` is a placeholder so the project compiles and the
server has something to serve.
