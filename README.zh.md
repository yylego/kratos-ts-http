<!-- TEMPLATE (ZH) BEGIN: BADGES -->
[![GitHub Workflow Status (branch)](https://img.shields.io/github/actions/workflow/status/yylego/kratos-ts-http/release.yml?branch=main&label=BUILD)](https://github.com/yylego/kratos-ts-http/actions/workflows/release.yml?query=branch%3Amain)
[![GoDoc](https://pkg.go.dev/badge/github.com/yylego/kratos-ts-http)](https://pkg.go.dev/github.com/yylego/kratos-ts-http)
[![Coverage Status](https://img.shields.io/coveralls/github/yylego/kratos-ts-http/main.svg)](https://coveralls.io/github/yylego/kratos-ts-http?branch=main)
[![Supported Go Versions](https://img.shields.io/badge/Go-1.25%2B-lightgrey.svg)](https://go.dev/)
[![GitHub Release](https://img.shields.io/github/release/yylego/kratos-ts-http.svg)](https://github.com/yylego/kratos-ts-http/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/yylego/kratos-ts-http)](https://goreportcard.com/report/github.com/yylego/kratos-ts-http)
<!-- TEMPLATE (ZH) CLOSE: BADGES -->

# kratos-ts-http

> 从 Kratos proto 文件生成【浏览器可直接用】的 TypeScript 客户端
> 一个 protoc 插件，产出 HTTP 形态的客户端，而不是 gRPC 形态
> 从 Go 后端到 TypeScript 前端的完整类型安全
> 像调用原生函数一样调用后端 API

---

![grpc-to-http-overview](https://raw.githubusercontent.com/yylego/grpc-to-http/main/assets/grpc-to-http-overview.svg)

---

<!-- TEMPLATE (ZH) BEGIN: LANGUAGE NAVIGATION -->

## 英文文档

[ENGLISH README](README.md)

<!-- TEMPLATE (ZH) CLOSE: LANGUAGE NAVIGATION -->

## 背景

`protoc-gen-ts`（来自 `@protobuf-ts/plugin`）把 proto 翻译成 TypeScript 这件事做得很好，但它产出的客户端说的是 **gRPC**，而浏览器不能原生发 gRPC。这些客户端必须先改写成 **HTTP** 形态，前端才能用。

把改写做成第二步，代价是：一个用完就扔的中间目录、Makefile 里一条改写命令、以及一个要在好几处重复写的目录名。改了一处忘了另一处，生成就会静默落到改写扫不到的地方 —— **而且照样成功退出**。

**本项目把改写折进了生成过程里。** 插件把请求转手给 `protoc-gen-ts`，拿回结果后就地把 `*.client.ts` 改写掉，然后才交还给 buf。所以 buf 落盘的**已经是最终代码**，输出目录是结果，不再是中间产物。

## 两种用法

| | 路线 | 入口 | 谁来驱动 buf |
|---|---|---|---|
| **①** | protoc 插件，一步到位 | `protoc-gen-kratos-ts-http` | 你的 Makefile / 命令行 |
| **②** | 嵌进你自己的 Go 程序 | `kratostshttp.BufGenerate(...)` | 你自己的代码 |

两条都能到终点，按你的习惯挑一条。

## 安装

三条命令。调用链是 buf → 本项目的插件 → protoc-gen-ts，所以【从底下往上装】：

```bash
# protoc-gen-ts 真正干 proto→TS 翻译 —— 【必需】，本项目会去调它
npm install -g @protobuf-ts/plugin

# 本项目的插件，负责把 protoc-gen-ts 的产物改写成 HTTP 形态
go install github.com/yylego/kratos-ts-http/cmd/protoc-gen-kratos-ts-http@latest

# buf 驱动整条链 —— Kratos v3 项目自带，先用 which buf 查一下
go install github.com/bufbuild/buf/cmd/buf@latest
```

`@protobuf-ts/plugin` **不是可选的**。本项目自己不翻译 proto，它把这一步转交给上游 —— [缘由](#设计说明)见下文。插件启动时会先检查，缺了就直接告诉你该装什么，而不是抛一句没给出路的 `executable file not found`。

## 用法一：protoc 插件

要两份配置文件。示例里都有实物，下面不是伪代码：
[`buf.yaml`](internal/examples/example1/buf.yaml) 和
[`buf.gen.ts.yaml`](internal/examples/example1/buf.gen.ts.yaml)。

**`buf.yaml`** 说明 proto 在哪、要拉哪些依赖。Kratos v3 项目自带一份，所以你可能只需要补上 `deps` 那行：

```yaml
version: v2
modules:
  - path: api
deps:
  - buf.build/googleapis/googleapis   # google.api.http 就是从这儿来的
```

然后跑一次 `buf dep update`。它会生成 `buf.lock`，把 googleapis 的具体提交钉死 —— 这样产物才不会因为上游变动而漂。**漏了这步，生成会直接报 `imported file does not exist`。**

**`buf.gen.ts.yaml`** 指定用哪个插件、带哪些选项：

```yaml
version: v2
inputs:
  - directory: api
plugins:
  - local: protoc-gen-kratos-ts-http
    out: .   # 落点【不在这儿定】—— 由命令行的 -o 决定
    opt:
      - ts_nocheck
      - eslint_disable
      - long_type_string
```

用 Makefile target 驱动它：

```makefile
kratos_ts_http:
	buf generate --template buf.gen.ts.yaml --include-imports -o ../web/src/rpc
```

**落点只由一个地方决定：命令行上的 `-o`。** 模板里保持 `out: .`，因为 buf 会把两者【拼起来】—— 两边写同一个路径，产物会套进自己里面一层。buf 会自己建目录，写进去的**就是最终客户端**，所以没有中转目录要拷贝，也没有第二条命令会被忘掉。

然后在前端装上运行时：

```bash
npm install @yylego/grpc-to-http
```

**`opt` 各项说明：**

| 配置项 | 作用 |
| --- | --- |
| `ts_nocheck` | 加上 `// @ts-nocheck`，生成的文件跳过类型检查 |
| `eslint_disable` | 加上 `/* eslint-disable */`，让 lint 工具别管它们 |
| `long_type_string` | 64 位整数映射成 `string` 而不是 `bigint`，这样过 JSON 不丢精度 |

`--include-imports` 让 buf 把 import 进来的 google 标准类型也一并生成，这样客户端的 import 才能解析。google protos 由 buf 从 buf.build 拉取（在 `buf.yaml` 的 `deps` 里声明 `buf.build/googleapis/googleapis`），**不需要本地的 `third_party` proto 目录**。

> 前端要是在另一个仓库里，就把 `-o` 指向一个 `.out` 结尾的中转目录：Go 官方的 `.gitignore` 模板本来就忽略 `*.out`，中转的代码不会混进后端仓库。

## 用法二：Go 代码驱动

```go
import "github.com/yylego/kratos-ts-http/kratostshttp"

// 三个参数：在哪儿跑、生成到哪儿、其余 buf 参数【原样透传】
err := kratostshttp.BufGenerate(
    "/path/to/the/kratos/project",   // workRoot：buf 的相对路径都相对它解析
    "/path/to/the/frontend/src/rpc", // outPath：对应 -o，直接就是最终落点
    []string{
        "--template", "buf.gen.ts.yaml",
        "--include-imports",
        "api",
    },
)
```

这样"后端改 proto → 前端拿到新客户端"就变成一次函数调用，中间没有目录要拷来拷去。

**为什么其余参数是原样透传的字符串数组、而不是一个结构体**：那些 flag 是 **buf 的词汇**，不是本项目的。逐个建模意味着 buf 每加一个 flag 本项目就得跟着加字段，还会把作者没想到的用法（`--path` / `--exclude-path` / `--type` …）悄悄挡在门外。透传零成本，控制权完全留给调用方。

⚠️ **注意 `-o` 与模板里 `out:` 的关系** —— `out:` 是**相对 `-o`** 的，两者会拼起来。想让产物正好落在 `outPath`，把模板里的 `out:` 写成 `.`。

⚠️ **`BufGenerate` 自己不做任何替换。** 它只负责跑 buf；替换之所以发生，是因为**模板里写的是** `protoc-gen-kratos-ts-http`。要是模板里配成 `protoc-gen-ts`，这次调用照样成功、照样返回 `nil`，产物却停在 gRPC 形态。这里的"成功"只意味着"buf 跑完了"，从来不意味着"目标达成了"。这是**刻意的分工** —— 模板你本来就得写，插件的选择早有天然归宿。

💡 `--config` / `--template` **既收文件路径，也直接收配置【内容】本身**，所以动态拼的配置可以直接塞进 `bufArgs`、不必落临时文件。⛔ 但 buf 区分二者，看的是【这串字符结尾像不像 `.yaml` / `.yml` / `.json`】，**跟文件存不存在无关** —— 所以内容末尾恰好以 `.yaml` 结尾（哪怕只是行尾注释提到某个文件名），整段就会被当文件名去 open，报出 `open version: v2` 这种完全指不到病因的错。传内容时留意最后一行。

## 示例

[`internal/examples/example1`](internal/examples/example1) 把整条链摆在一个目录里，顺序就是问题冒出来的顺序：

| 文件 | 它回答什么 |
|---|---|
| [`api/greeter/v1/greeter.proto`](internal/examples/example1/api/greeter/v1/greeter.proto) | 假设你的 API 长这样。那行 `google.api.http` 把 SayHello 映射到 `POST /v1/greeter/hello` |
| [`buf.yaml`](internal/examples/example1/buf.yaml) + [`buf.lock`](internal/examples/example1/buf.lock) | proto 在哪、拉哪些依赖。lock 把 googleapis 的具体提交钉死 |
| [`buf.gen.ts.yaml`](internal/examples/example1/buf.gen.ts.yaml) | 用哪个插件。`out: .` —— 落点不在这儿定 |
| [`Makefile`](internal/examples/example1/Makefile) | 先 `install`，再 `generate`，然后 `check` |
| [`web/src/rpc/`](internal/examples/example1/web/src/rpc) | 客户端落在这儿：就在前端里，中间没有中转目录 |
| [`golden.ts`](internal/examples/example1/golden.ts) | **你最终会拿到的客户端** |

把 `golden.ts` 读一遍，不用跑任何东西就明白了。想自己跑，在那个目录里 `make generate` 就能复现，`make check` 拿两者对比。

## 设计说明

产物分两类。客户端文件（`*.client.ts`，每个约 57 行）是唯一值得动的；消息类型文件（`*.ts`，每个约 250 行、数量还多出好几倍）装着 `@protobuf-ts/runtime` 的完整序列化实现 —— 自己重写就要跟那个运行时的内部约定**逐字节对齐、而且永远跟着它升级**，一旦出现偏差，前端运行时便会出错。

转手给它、只改我们真正关心的那几行，才是划算又安全的做法。

## 对外接口

```go
func RunTsPlugin(stdin io.Reader, stdout io.Writer) (GenStats, error)  // 完成一次完整的 protoc 插件交互
func BufGenerate(workRoot, outPath string, bufArgs []string) error  // 执行一次 `buf generate`

type GenStats struct{ FileCount, EditCount int }  // 跑完一趟交回的结果
```

**就这三个，两条路线各一个、外加一个统计。** 改写规则本身留在包内，供两条路线共用，不对外承诺任何契约。

改写是**幂等**的，所以跑在已经转换过的产物上也不会改坏。

至于**存量转换** —— 手上已经有一批落在磁盘上的 gRPC 形态客户端要改，那是另一码事，[kratos-vue3](https://github.com/yylego/kratos-vue3) 的 `vue3kratos` 命令一直提供这个能力，用它就好。

## 相关项目

- [grpc-to-http](https://github.com/yylego/grpc-to-http) —— npm 包 [`@yylego/grpc-to-http`](https://www.npmjs.com/package/@yylego/grpc-to-http)，生成的客户端 import 的就是它；负责把 protobuf-ts 的 gRPC 调用转成基于 Axios 的 HTTP 请求
- [protobuf-ts](https://github.com/timostamm/protobuf-ts) —— 上游的 `@protobuf-ts/plugin`，本项目转手给它

## 项目由来

本项目脱胎于 [kratos-vue3](https://github.com/yylego/kratos-vue3)。那一版把改写做成了独立的第二步，名字里还挂着一个产物本身并不成立的框架名。这里的改写规则是它的直系后代，如今折进了生成过程，也换上了一个描述**产物**、而不是描述**作者用什么前端**的名字。

`kratos-vue3` 会继续留着、继续可用。新项目从这里开始就好。

---

<!-- TEMPLATE (ZH) BEGIN: STANDARD PROJECT FOOTER -->
<!-- VERSION 2025-11-25 03:52:28.131064 +0000 UTC -->

## 📄 许可证类型

MIT 许可证 - 详见 [LICENSE](LICENSE)。

---

## 💬 联系与反馈

非常欢迎贡献代码！报告 BUG、建议功能、贡献代码：

- 🐛 **问题报告？** 在 GitHub 上提交问题并附上重现步骤
- 💡 **新颖思路？** 创建 issue 讨论
- 📖 **文档疑惑？** 报告问题，帮助我们完善文档
- 🚀 **需要功能？** 分享使用场景，帮助理解需求
- ⚡ **性能瓶颈？** 报告慢操作，协助解决性能问题
- 🔧 **配置困扰？** 询问复杂设置的相关问题
- 📢 **关注进展？** 关注仓库以获取新版本和功能
- 🌟 **成功案例？** 分享这个包如何改善工作流程
- 💬 **反馈意见？** 欢迎提出建议和意见

---

## 🔧 代码贡献

新代码贡献，请遵循此流程：

1. **Fork**：在 GitHub 上 Fork 仓库（使用网页界面）
2. **克隆**：克隆 Fork 的项目（`git clone https://github.com/yourname/repo-name.git`）
3. **导航**：进入克隆的项目（`cd repo-name`）
4. **分支**：创建功能分支（`git checkout -b feature/xxx`）
5. **编码**：实现您的更改并编写全面的测试
6. **测试**：（Golang 项目）确保测试通过（`go test ./...`）并遵循 Go 代码风格约定
7. **文档**：面向用户的更改需要更新文档
8. **暂存**：暂存更改（`git add .`）
9. **提交**：提交更改（`git commit -m "Add feature xxx"`）确保向后兼容的代码
10. **推送**：推送到分支（`git push origin feature/xxx`）
11. **PR**：在 GitHub 上打开 Merge Request（在 GitHub 网页上）并提供详细描述

请确保测试通过并包含相关的文档更新。

---

## 🌟 项目支持

非常欢迎通过提交 Merge Request 和报告问题来贡献此项目。

**项目支持：**

- ⭐ **给予星标**如果项目对您有帮助
- 🤝 **分享项目**给团队成员和（golang）编程朋友
- 📝 **撰写博客**关于开发工具和工作流程 - 我们提供写作支持
- 🌟 **加入生态** - 致力于支持开源和（golang）开发场景

**祝你用这个包编程愉快！** 🎉🎉🎉

<!-- TEMPLATE (ZH) CLOSE: STANDARD PROJECT FOOTER -->

---

<!-- TEMPLATE (ZH) BEGIN: GITHUB STARS -->
## GitHub 标星点赞

[![Stargazers](https://starchart.cc/yylego/kratos-ts-http.svg?variant=adaptive)](https://starchart.cc/yylego/kratos-ts-http)
<!-- TEMPLATE (ZH) CLOSE: GITHUB STARS -->
