package kratostshttp

import (
	"strings"

	"github.com/yylego/erero"
)

// This file holds the rewrite rules themselves, which both routes share.
// 本文件包含改写规则本身，两条路线都依赖它。

// clientFileSuffix marks which generated files are the gRPC clients we rewrite
// clientFileSuffix 标记哪些生成文件是需要改写的 gRPC 客户端
const clientFileSuffix = ".client.ts"

// rewriteCode converts gRPC TS client code to HTTP client code, idempotent across repeat runs
// rewriteCode 把 gRPC 形态的 TS 客户端代码改写为 HTTP 形态，重复运行保持幂等
//
// It returns an error when the rewrite has run but the grpc-to-http import cannot be placed,
// instead of emitting TS that uses symbols it does not import.
// 当改写已完成、但 grpc-to-http 导入无处可插时会返回错误，而不是产出"用了却没 import"的错误 TS。
func rewriteCode(srcContent string) (string, error) {
	newContent := srcContent

	// Replace gRPC invocations with HTTP equivalents
	// 把代码中调用 grpc 的地方改成调用 http
	newContent = strings.ReplaceAll(newContent, "stackIntercept<", "executeGrpcToHttp<")
	newContent = strings.ReplaceAll(newContent, "UnaryCall<", "GrpcToHttpPromise<")

	targetImport := `import { executeGrpcToHttp } from '@yylego/grpc-to-http';` +
		"\n" +
		`import type { GrpcToHttpPromise } from '@yylego/grpc-to-http';`

	// The import is needed once the code leans on the http symbols. A file with none of them
	// (a non-client file, and likewise one with no gRPC calls) passes through untouched.
	// 仅当代码用到这两个 http 符号时才需要导入；一个都不含的文件（非 client 文件、或本就没
	// gRPC 调用）原样透传。
	if !strings.Contains(newContent, "executeGrpcToHttp") && !strings.Contains(newContent, "GrpcToHttpPromise") {
		return newContent, nil
	}

	// The import is in place -> a repeat run lands here, so it stays idempotent.
	// 已经导入过 -> 重复运行会走到这里，保持幂等。
	if strings.Contains(newContent, targetImport) {
		return newContent, nil
	}

	// Place the new import next to protobuf-ts own RpcOptions import.
	// 把新导入插在 protobuf-ts 自己的 RpcOptions 导入旁边。
	searchImport := `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";`
	targetIndex := strings.Index(newContent, searchImport)
	if targetIndex == -1 {
		// The replacements above put executeGrpcToHttp / GrpcToHttpPromise into the code, but
		// the landmark import is gone, so the new import cannot be placed. Emitting the file now
		// would produce TS that uses symbols it does not import, so return an error instead.
		// 上面已经把 executeGrpcToHttp / GrpcToHttpPromise 写进代码，但地标导入已不在，新导入
		// 无处可插；此时再产出文件，会得到"用了却没 import"的错误 TS，因此这里返回错误。
		return "", erero.Errorf("rewrote gRPC calls to HTTP form but cannot add the @yylego/grpc-to-http import: landmark %q not found (did @protobuf-ts change its output?)", searchImport)
	}

	// Insert the new import following the landmark.
	// 在地标之后插入新导入。
	insertIndex := targetIndex + len(searchImport)
	newContent = newContent[:insertIndex] + "\n" + targetImport + newContent[insertIndex:]
	return newContent, nil
}
