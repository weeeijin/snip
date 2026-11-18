# snip

A small URL shortener written in Go: the application behind a hands-on DevOps / SRE learning project.

## API

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/links` | Body `{"url": "https://..."}` → `201` with the new link and its `code` |
| `GET` | `/{code}` | `302` redirect to the original URL, counts a click |
| `GET` | `/api/links/{code}` | Link details, including the click count |
| `GET` | `/healthz` | Liveness: the process is running |
| `GET` | `/readyz` | Readiness: storage is reachable (`503` if not) |

## Run locally

```bash
go run ./cmd/snip                      # in-memory store (data lost on restart)

# With PostgreSQL
DATABASE_URL='postgres://user:pass@localhost:5432/snip?sslmode=disable' go run ./cmd/snip
```

```bash
curl -X POST localhost:8080/api/links -d '{"url":"https://github.com"}'
curl -i localhost:8080/<code>
```

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | Port to listen on |
| `DATABASE_URL` | *(unset)* | PostgreSQL URL; unset uses an in-memory store |

## Test

```bash
go test -race -cover ./...
```

## Layout

```
cmd/snip/          entry point: config, graceful shutdown
internal/server/   HTTP routes, validation, responses (+ tests)
internal/store/    Store interface, in-memory and PostgreSQL implementations
```
