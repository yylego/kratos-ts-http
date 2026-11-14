package kratostshttp

import (
	"regexp"
	"strings"

	"github.com/yylego/erero"
)

// This file holds the rewrite rules themselves, which both routes share.
// 本文件包含改写规则本身，两条路线都依赖它。

// clientFileSuffix marks which generated files are the gRPC clients we rewrite
// clientFileSuffix 标记哪些生成文件是需要改写的 gRPC 客户端
const clientFileSuffix = ".client.ts"

// grpcToHttpModule is the npm package the rewritten clients import, quoted as it appears in the import
// grpcToHttpModule 是改写后的客户端导入的 npm 包，带着 import 里的引号
const grpcToHttpModule = `'@yylego/grpc-to-http'`

// unaryInvoke matches the protobuf-ts unary invocation up to its first argument, capturing the type arguments
// unaryInvoke 匹配 protobuf-ts 一元调用开头到第一个参数之前的部分，捕获类型参数
var unaryInvoke = regexp.MustCompile(`stackIntercept<([^>]*)>\("unary", `)

// serverStreamingInvoke matches the protobuf-ts server-streaming invocation up to the method argument,
// capturing the type arguments
// serverStreamingInvoke 匹配 protobuf-ts 服务端流式调用开头到 method 参数之前的部分，捕获类型参数
var serverStreamingInvoke = regexp.MustCompile(`stackIntercept<([^>]*)>\("serverStreaming", this\._transport, `)

// rewriteCode converts gRPC TS client code to HTTP client code, idempotent across repeat runs
// rewriteCode 把 gRPC 形态的 TS 客户端代码改写为 HTTP 形态，重复运行保持幂等
//
// It returns an error when the rewrite has run but the grpc-to-http import cannot be placed,
// instead of emitting TS that uses symbols it does not import.
// 当改写已完成、但 grpc-to-http 导入无处可插时会返回错误，而不是产出"用了却没 import"的错误 TS。
func rewriteCode(srcContent string) (string, error) {
	newContent := srcContent

	// Replace the unary and server-streaming invocations with HTTP equivalents; the stream function
	// takes no callType and no transport. Client-streaming and duplex methods keep the gRPC form:
	// Kratos serves them through WebSocket, which grpc-to-http does not handle, so a request gets the
	// transport's own message instead of going out as a wrong HTTP request.
	// 把一元调用和服务端流式调用改成 http 形态；流式函数不带 callType 和 transport。客户端流式和
	// 双向流式保持 gRPC 形态：Kratos 用 WebSocket 提供它们，grpc-to-http 不涵盖，保持原样时调用会
	// 由传输层给出清楚的提示，而不是发出一个错误的 HTTP 请求。
	newContent = unaryInvoke.ReplaceAllString(newContent, `executeGrpcToHttp<$1>("unary", `)
	newContent = strings.ReplaceAll(newContent, "UnaryCall<", "GrpcToHttpPromise<")
	newContent = serverStreamingInvoke.ReplaceAllString(newContent, "executeGrpcToHttpStream<$1>(")
	newContent = strings.ReplaceAll(newContent, "ServerStreamingCall<", "GrpcToHttpStream<")

	// Import just the http symbols the code uses. A file with none of them (a non-client file,
	// and likewise one with no gRPC calls) passes through untouched.
	// 只导入代码用到的 http 符号；一个都不含的文件（非 client 文件、或本就没 gRPC 调用）原样透传。
	var valueNames, typeNames []string
	for _, name := range []string{"executeGrpcToHttp", "executeGrpcToHttpStream"} {
		if strings.Contains(newContent, name+"<") {
			valueNames = append(valueNames, name)
		}
	}
	for _, name := range []string{"GrpcToHttpPromise", "GrpcToHttpStream"} {
		if strings.Contains(newContent, name+"<") {
			typeNames = append(typeNames, name)
		}
	}
	if len(valueNames) == 0 && len(typeNames) == 0 {
		return newContent, nil
	}

	// A repeat run finds the grpc-to-http imports of the last run: drop them and write them again
	// from the names in use now, so the result stays the same and a name added since then gets in.
	// 重复运行会碰到上一次写的 grpc-to-http 导入：先拿掉，再按现在用到的符号重新写，
	// 这样结果不变，期间新用到的符号也能补上。
	newContent = dropGrpcToHttpImports(newContent)

	var importLines []string
	if len(valueNames) > 0 {
		importLines = append(importLines, "import { "+strings.Join(valueNames, ", ")+" } from "+grpcToHttpModule+";")
	}
	if len(typeNames) > 0 {
		importLines = append(importLines, "import type { "+strings.Join(typeNames, ", ")+" } from "+grpcToHttpModule+";")
	}
	targetImport := strings.Join(importLines, "\n")

	// Place the new import next to protobuf-ts own RpcOptions import.
	// 把新导入插在 protobuf-ts 自己的 RpcOptions 导入旁边。
	searchImport := `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";`
	targetIndex := strings.Index(newContent, searchImport)
	if targetIndex == -1 {
		// The replacements above put the grpc-to-http symbols into the code, but the landmark
		// import is gone, so the new import cannot be placed. Emitting the file now would produce
		// TS that uses symbols it does not import, so return an error instead.
		// 上面已经把 grpc-to-http 的符号写进代码，但地标导入已不在，新导入无处可插；此时再产出
		// 文件，会得到"用了却没 import"的错误 TS，因此这里返回错误。
		return "", erero.Errorf("rewrote gRPC calls to HTTP form but cannot add the @yylego/grpc-to-http import: landmark %q not found (did @protobuf-ts change its output?)", searchImport)
	}

	// Insert the new import following the landmark.
	// 在地标之后插入新导入。
	insertIndex := targetIndex + len(searchImport)
	newContent = newContent[:insertIndex] + "\n" + targetImport + newContent[insertIndex:]
	return newContent, nil
}

// dropGrpcToHttpImports removes the import lines of grpc-to-http, the ones rewriteCode writes
// dropGrpcToHttpImports 去掉 grpc-to-http 的导入行，也就是 rewriteCode 写进去的那几行
func dropGrpcToHttpImports(content string) string {
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "import ") && strings.HasSuffix(line, " from "+grpcToHttpModule+";") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
