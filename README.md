<!-- TEMPLATE (EN) BEGIN: BADGES -->

[![GitHub Workflow Status (branch)](https://img.shields.io/github/actions/workflow/status/yylego/kratos-ts-http/release.yml?branch=main&label=BUILD)](https://github.com/yylego/kratos-ts-http/actions/workflows/release.yml?query=branch%3Amain)
[![GoDoc](https://pkg.go.dev/badge/github.com/yylego/kratos-ts-http)](https://pkg.go.dev/github.com/yylego/kratos-ts-http)
[![Coverage Status](https://img.shields.io/coveralls/github/yylego/kratos-ts-http/main.svg)](https://coveralls.io/github/yylego/kratos-ts-http?branch=main)
[![Supported Go Versions](https://img.shields.io/badge/Go-1.25%2B-lightgrey.svg)](https://go.dev/)
[![GitHub Release](https://img.shields.io/github/release/yylego/kratos-ts-http.svg)](https://github.com/yylego/kratos-ts-http/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/yylego/kratos-ts-http)](https://goreportcard.com/report/github.com/yylego/kratos-ts-http)

<!-- TEMPLATE (EN) CLOSE: BADGES -->

# kratos-ts-http

> Generate web-safe TypeScript clients from Kratos proto files
> A protoc plugin that emits HTTP-form clients instead of gRPC
> Complete type-safe code from the Go backend to the TypeScript frontend
> Invoke backend APIs just as native functions

---

![grpc-to-http-overview](https://raw.githubusercontent.com/yylego/grpc-to-http/main/assets/grpc-to-http-overview.svg)

---

<!-- TEMPLATE (EN) BEGIN: LANGUAGE NAVIGATION -->

## CHINESE README

[中文说明](README.zh.md)

<!-- TEMPLATE (EN) CLOSE: LANGUAGE NAVIGATION -->

## Background

`protoc-gen-ts` (from `@protobuf-ts/plugin`) turns proto files into excellent TypeScript, but the clients it emits speak **gRPC**, which has no native home in the web. Those clients must be rewritten into **HTTP** form before a frontend can use them.

Doing that as a second pass costs a scratch DIR, a rewrite command in the Makefile, and one DIR name repeated in more than one place. Change one, miss the next, and generation lands somewhere the rewrite does not look — and exits with success.

**This package folds the rewrite into generation.** The plugin hands the request to `protoc-gen-ts`, rewrites the `*.client.ts` files it gets back, then returns to buf. What buf writes to disk is the finished code, so the output DIR is a result instead of an intermediate.

## Two Ways to Use

|       | Route                    | Entrance                        | Who drives buf              |
| ----- | ------------------------ | ------------------------------- | --------------------------- |
| **①** | protoc plugin, one step  | `protoc-gen-kratos-ts-http`     | The Makefile / command line |
| **②** | Embedded in a Go program | `kratostshttp.BufGenerate(...)` | The Go code itself          |

Both end at the same place, so take the one that suits the workflow.

## Rewrite Rules

Each method in a `*.client.ts` file is rewritten according to its kind:

| Kind                     | Generated code                                                                                                                                                                            |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unary                    | `executeGrpcToHttp(...)`, returning `GrpcToHttpPromise<I, O>`                                                                                                                             |
| Server streaming         | `executeGrpcToHttpStream(method, opt, input)`, returning `GrpcToHttpStream<I, O>`, read with `for await`                                                                                  |
| Client streaming, duplex | `executeGrpcToHttpWebSocket(method, opt)`, returning `GrpcToHttpWebSocket<I, O>`: `send` and `closeSend` to write, `for await` to read, through the WebSocket that Kratos serves these on |

## Installation

Three commands. The chain runs buf → this package's plugin → protoc-gen-ts, so set it up from the bottom up:

```bash
# protoc-gen-ts does the proto→TS translation -- REQUIRED, this package invokes it
npm install -g @protobuf-ts/plugin

# this package's plugin, which rewrites what protoc-gen-ts produces
go install github.com/yylego/kratos-ts-http/cmd/protoc-gen-kratos-ts-http@latest

# buf drives the whole thing -- Kratos v3 projects ship it, check with: which buf
go install github.com/bufbuild/buf/cmd/buf@latest
```

`@protobuf-ts/plugin` is **required**. This package does not translate protos on its own, it delegates that work — see [the reason](#design-notes). The plugin checks at startup and names what is missing, instead of dying with a bare `executable file not found`.

## Usage 1: protoc Plugin

Two config files. Live copies of both sit in the example, so nothing here is pseudo code: [`buf.yaml`](internal/examples/example1/buf.yaml) and [`buf.gen.ts.yaml`](internal/examples/example1/buf.gen.ts.yaml).

**`buf.yaml`** says where the protos live and which deps to fetch. Kratos v3 projects ship one, so this might just mean adding the `deps` line:

```yaml
version: v2
modules:
  - path: api
deps:
  - buf.build/googleapis/googleapis # google.api.http comes from here
```

Run `buf dep update` once afterwards. It writes a `buf.lock` that pins the exact googleapis commit, which keeps the generated output stable. Skip this step and generation stops with `imported file does not exist`.

**`buf.gen.ts.yaml`** names the plugin and its options:

```yaml
version: v2
inputs:
  - directory: api
plugins:
  - local: protoc-gen-kratos-ts-http
    out: . # the destination is NOT set here, -o on the command line decides it
    opt:
      - ts_nocheck
      - eslint_disable
      - long_type_string
```

Drive it from a Makefile target:

```makefile
kratos_ts_http:
	buf generate --template buf.gen.ts.yaml --include-imports -o ../web/src/rpc
```

**One place decides the destination: the `-o` on the command line.** Keep `out: .` in the template, because buf JOINS the two and naming the same path twice nests the output inside itself. buf creates the DIRs it needs, and what it writes there is the finished client, so there is no staging DIR to take from and no second command to forget.

Then add the runtime to the frontend:

```bash
npm install @yylego/grpc-to-http
```

Server-streaming methods need `@yylego/grpc-to-http` 0.0.2 and up, client-streaming and duplex methods 0.0.5 and up. The Kratos backend must use the protojson codec as its `json` codec, see [Data Format](https://github.com/yylego/grpc-to-http#data-format).

**`opt` options:**

| Option             | Effect                                                                    |
| ------------------ | ------------------------------------------------------------------------- |
| `ts_nocheck`       | Adds `// @ts-nocheck`, so generated files skip type checking              |
| `eslint_disable`   | Adds `/* eslint-disable */`, so lint tools leave them alone               |
| `long_type_string` | Maps 64-bit integers to `string` instead of `bigint`, which survives JSON |

`--include-imports` makes buf emit the imported google standard types too, so the client imports resolve. buf pulls those from buf.build once `buf.build/googleapis/googleapis` is declared in `deps` inside `buf.yaml`, so no `third_party` proto tree is needed on disk.

> Should the frontend live in a separate repo, aim `-o` at a staging DIR named with a `.out` suffix: Go's standard `.gitignore` ignores `*.out`, so the staged code stays out of the backend repo.

## Usage 2: Drive from Go

```go
import "github.com/yylego/kratos-ts-http/kratostshttp"

// Three params: where to run, where output lands, everything else passed to buf AS IS
err := kratostshttp.BufGenerate(
    "/path/to/the/kratos/project",   // workRoot: buf resolves its relative paths against this
    "/path/to/the/frontend/src/rpc", // outPath: the `-o` value, the end destination
    []string{
        "--template", "buf.gen.ts.yaml",
        "--include-imports",
        "api",
    },
)
```

This turns "backend edits a proto → frontend has a fresh client" into one function invocation, with no intermediate DIR in between.

**What makes the rest a passed-through string slice instead of a struct:** those flags are buf's terms, not ours. Modelling them would mean adding a field each time buf grows one, and would shut out anything we did not think of (`--path`, `--exclude-path`, `--type`, …). Passing them through costs nothing and leaves the calling code in charge.

⚠️ **Mind how `-o` relates to the template's `out:`** — `out:` resolves relative to `-o` and the two are joined. To land output at `outPath` itself, write `out: .` in the template.

⚠️ **`BufGenerate` rewrites nothing itself.** It runs buf; the rewrite happens because the template names `protoc-gen-kratos-ts-http`. Name `protoc-gen-ts` there instead and this invocation succeeds just the same and returns `nil`, while the output stays in gRPC form. Success here means "buf ran", not "the aim was reached". That split is deliberate — a template gets written regardless, so the plugin choice has a home of its own.

💡 `--config` and `--template` accept **a file path, and also the config content itself**, so a config built at run time can go straight into `bufArgs` with no temp file. ⛔ But buf tells the two apart from how the string LOOKS at its end (`.yaml` / `.yml` / `.json`), **not** from the file existing on disk — so content ending in `.yaml` (even inside a trailing comment) gets opened as a filename and fails with something like `open version: v2`, which points nowhere close to the cause. Mind the last line when passing content.

## Example

[`internal/examples/example1`](internal/examples/example1) puts the whole chain in one DIR, in the sequence the questions come up:

| File                                                                                                  | What it answers                                                                                                                                                                    |
| ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`api/greeter/v1/greeter.proto`](internal/examples/example1/api/greeter/v1/greeter.proto)             | Suppose the API looks like this. The `google.api.http` lines map SayHello onto `POST /v1/greeter/hello`, and the server-streaming StreamHello onto `POST /v1/greeter/hello/stream` |
| [`buf.yaml`](internal/examples/example1/buf.yaml) + [`buf.lock`](internal/examples/example1/buf.lock) | Which protos to read and which deps to fetch. The lock pins the exact googleapis commit                                                                                            |
| [`buf.gen.ts.yaml`](internal/examples/example1/buf.gen.ts.yaml)                                       | Which plugin to run. `out: .` keeps the destination out of the template                                                                                                            |
| [`Makefile`](internal/examples/example1/Makefile)                                                     | `install`, then `generate`, then `check`                                                                                                                                           |
| [`web/src/rpc/`](internal/examples/example1/web/src/rpc)                                              | Where the client lands: inside the frontend, with no staging DIR between                                                                                                           |
| [`golden.ts`](internal/examples/example1/golden.ts)                                                   | **The client you end up with**                                                                                                                                                     |

Read `golden.ts` and the whole thing makes sense without running anything. `make generate` inside that DIR reproduces it, and `make check` diffs the two.

## Design Notes

The output splits in two. Client files (`*.client.ts`, about 57 lines each) are the sole ones worth touching. Message files (`*.ts`, about 250 lines each, with more files besides) contain `@protobuf-ts/runtime`'s complete serialization implementation — rewriting those would mean matching that runtime's internals byte to byte, from release to release, and drift crashes the frontend at run time.

Delegating, then rewriting just the few lines we care about, is the cheap and sound trade.

## Package API

```go
func RunTsPlugin(stdin io.Reader, stdout io.Writer) (GenStats, error)  // one complete protoc-plugin exchange
func BufGenerate(workRoot, outPath string, bufArgs []string) error  // run one `buf generate`

type GenStats struct{ FileCount, EditCount int }  // what one run reports back
```

Three symbols, no more: one to each route, plus the counts. The rewrite rules themselves remain inside the package, serving both routes and exposing no contract.

The rewrite is **idempotent**, so a repeat pass across converted output is safe.

Converting a batch of gRPC-form clients that sit on disk is a separate job, and [kratos-vue3](https://github.com/yylego/kratos-vue3) ships the `vue3kratos` command to do just that.

## Related Projects

- [grpc-to-http](https://github.com/yylego/grpc-to-http) — npm package [`@yylego/grpc-to-http`](https://www.npmjs.com/package/@yylego/grpc-to-http), the runtime the generated clients import; converts protobuf-ts gRPC invocations into Axios HTTP requests
- [protobuf-ts](https://github.com/timostamm/protobuf-ts) — the upstream `@protobuf-ts/plugin` this package delegates to

## Lineage

This package grew out of [kratos-vue3](https://github.com/yylego/kratos-vue3), which worked the rewrite as a separate second pass and carried a framework name the generated code did not earn. The rewrite rules here are its direct descendants, now folded into generation, and the package wears a name that describes the output instead of the frontend we happen to use.

`kratos-vue3` stays up and keeps working. New projects should start here.

---

<!-- TEMPLATE (EN) BEGIN: STANDARD PROJECT FOOTER -->
<!-- VERSION 2025-11-25 03:52:28.131064 +0000 UTC -->

## 📄 License

MIT License - see [LICENSE](LICENSE).

---

## 💬 Contact & Feedback

Contributions are welcome! Report bugs, suggest features, and contribute code:

- 🐛 **Mistake reports?** Open an issue on GitHub with reproduction steps
- 💡 **Fresh ideas?** Create an issue to discuss
- 📖 **Documentation confusing?** Report it so we can improve
- 🚀 **Need new features?** Share the use cases to help us understand requirements
- ⚡ **Performance issue?** Help us optimize through reporting slow operations
- 🔧 **Configuration problem?** Ask questions about complex setups
- 📢 **Follow project progress?** Watch the repo to get new releases and features
- 🌟 **Success stories?** Share how this package improved the workflow
- 💬 **Feedback?** We welcome suggestions and comments

---

## 🔧 Development

New code contributions, follow this process:

1. **Fork**: Fork the repo on GitHub (using the webpage UI).
2. **Clone**: Clone the forked project (`git clone https://github.com/yourname/repo-name.git`).
3. **Navigate**: Navigate to the cloned project (`cd repo-name`)
4. **Branch**: Create a feature branch (`git checkout -b feature/xxx`).
5. **Code**: Implement the changes with comprehensive tests
6. **Testing**: (Golang project) Ensure tests pass (`go test ./...`) and follow Go code style conventions
7. **Documentation**: Update documentation to support client-facing changes
8. **Stage**: Stage changes (`git add .`)
9. **Commit**: Commit changes (`git commit -m "Add feature xxx"`) ensuring backward compatible code
10. **Push**: Push to the branch (`git push origin feature/xxx`).
11. **PR**: Open a merge request on GitHub (on the GitHub webpage) with detailed description.

Please ensure tests pass and include relevant documentation updates.

---

## 🌟 Support

Welcome to contribute to this project via submitting merge requests and reporting issues.

**Project Support:**

- ⭐ **Give GitHub stars** if this project helps you
- 🤝 **Share with teammates** and (golang) programming friends
- 📝 **Write tech blogs** about development tools and workflows - we provide content writing support
- 🌟 **Join the ecosystem** - committed to supporting open source and the (golang) development scene

**Have Fun Coding with this package!** 🎉🎉🎉

<!-- TEMPLATE (EN) CLOSE: STANDARD PROJECT FOOTER -->
