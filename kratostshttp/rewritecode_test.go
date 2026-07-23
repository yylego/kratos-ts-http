package kratostshttp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// grpcClientSnippet is a compact protobuf-ts gRPC client, just enough to exercise each
// replacement rule (the import site, the UnaryCall return type, the stackIntercept invocation).
//
// grpcClientSnippet 是一段最小的 protobuf-ts gRPC 客户端，刚好覆盖每条替换规则
// （import 地标、UnaryCall 返回类型、stackIntercept 调用）
const grpcClientSnippet = `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";
export interface IFooClient {
    bar(input: BarRequest, options?: RpcOptions): UnaryCall<BarRequest, BarReply>;
}
export class FooClient implements IFooClient {
    bar(input: BarRequest, options?: RpcOptions): UnaryCall<BarRequest, BarReply> {
        return stackIntercept<BarRequest, BarReply>("unary", this._transport, method, opt, input);
    }
}
`

// requireConverted asserts the content is a converted HTTP client.
//
// requireConverted 断言内容已转换成 HTTP 客户端
func requireConverted(t *testing.T, out string) {
	t.Helper()
	require.Contains(t, out, "import { executeGrpcToHttp } from '@yylego/grpc-to-http';")
	require.Contains(t, out, "executeGrpcToHttp<")
	require.Contains(t, out, "GrpcToHttpPromise<")
	require.NotContains(t, out, "stackIntercept<")
	require.NotContains(t, out, "UnaryCall<")
}

func TestRewriteCode(t *testing.T) {
	out, err := rewriteCode(grpcClientSnippet)
	require.NoError(t, err)
	requireConverted(t, out)

	// Running again keeps the same result (idempotent).
	// 再跑一次结果不变（幂等）
	twice, err := rewriteCode(out)
	require.NoError(t, err)
	require.Equal(t, out, twice)
}

// TestRewriteCodeMissingLandmarkErrors guards the defensive check: code with a gRPC invocation
// but no RpcOptions landmark gets its invocations rewritten yet cannot receive the import, so
// the function must error instead of emitting TS that uses symbols it does not import.
//
// 守护防御逻辑：有 gRPC 调用、却没有 RpcOptions 地标导入的代码，调用被替换了、导入却插不进去，
// 此时必须报错，而不是产出"用了却没 import"的坏 TS。
func TestRewriteCodeMissingLandmarkErrors(t *testing.T) {
	const noLandmark = `export function foo() {
    return stackIntercept<BarRequest, BarReply>("unary", this._transport, method, opt, input);
}
`
	_, err := rewriteCode(noLandmark)
	require.Error(t, err)
	require.Contains(t, err.Error(), "landmark")
}

// TestRewriteCodePassesNonClientThrough guards that content with no gRPC symbols comes back
// untouched and without an error (a non-client file, and likewise one with no gRPC calls).
//
// 守护：不含 gRPC 符号的内容原样返回、且不报错（非 client 文件、或本就没 gRPC 调用的文件）。
func TestRewriteCodePassesNonClientThrough(t *testing.T) {
	const plain = "export const count = 42;\n"
	out, err := rewriteCode(plain)
	require.NoError(t, err)
	require.Equal(t, plain, out)
}
