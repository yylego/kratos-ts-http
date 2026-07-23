// Command protoc-gen-kratos-ts-http generates web-safe TypeScript clients in one step.
//
// protoc-gen-kratos-ts-http 一步生成可在浏览器中直接使用的 TypeScript 客户端。
//
// It is a standard protoc plugin, so buf invokes it just as it invokes the rest. Point buf
// at it and the code buf writes to disk is the finished HTTP-form client, with no separate
// rewrite step afterwards.
// 它就是个标准 protoc 插件，buf 照常调它。把 buf 指向它，buf 落盘的就已经是
// 最终的 HTTP 形态客户端，后面不用再跑一遍改写命令。
//
// Usage: swap the plugin name in buf.gen.yaml, leave everything else alone.
// 用法：在 buf.gen.yaml 里把插件名换成它，其余不动。
//
//	plugins:
//	  - local: protoc-gen-kratos-ts-http
//	    out: <输出目录>
//	    opt: [ts_nocheck, eslint_disable, long_type_string]
//
// The work itself lives in kratostshttp.RunTsPlugin; this command is a thin entrance.
// Background -- what makes it delegate instead of generating TS itself, and how that saves a
// step -- see the README.
// 真正的实现在 kratostshttp.RunTsPlugin 里，本命令只是入口。背景（为什么转手而非自己生成
// TS、这样如何省掉一步）见 README。
package main

import (
	"fmt"
	"os"

	"github.com/yylego/kratos-ts-http/kratostshttp"
)

func main() {
	// stdin carries the CodeGeneratorRequest and stdout carries the response, so each
	// message we emit goes to stderr, see the warning in tsplugin.go
	// stdin 载着 CodeGeneratorRequest、stdout 载着响应，
	// 所以我们自己的每一句话都走 stderr，原因见 tsplugin.go 里的警告
	stats, err := kratostshttp.RunTsPlugin(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-kratos-ts-http: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "protoc-gen-kratos-ts-http: %d files seen, %d clients rewritten to HTTP form\n",
		stats.FileCount, stats.EditCount)
}
