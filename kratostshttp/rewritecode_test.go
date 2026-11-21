package kratostshttp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// grpcClientSnippet is a compact protobuf-ts gRPC client, just enough to exercise each
// replacement rule (the import site, and the return type and invocation of each kind of method).
//
// grpcClientSnippet 是一段最小的 protobuf-ts gRPC 客户端，刚好覆盖每条替换规则
// （import 地标，以及四种调用各自的返回类型和调用写法）
const grpcClientSnippet = `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";
export class FooClient implements IFooClient {
    send(input: FooParam, options?: RpcOptions): UnaryCall<FooParam, FooResp> {
        return stackIntercept<FooParam, FooResp>("unary", this._transport, method, opt, input);
    }
    watch(input: FooParam, options?: RpcOptions): ServerStreamingCall<FooParam, FooResp> {
        return stackIntercept<FooParam, FooResp>("serverStreaming", this._transport, method, opt, input);
    }
    upload(options?: RpcOptions): ClientStreamingCall<FooParam, FooResp> {
        return stackIntercept<FooParam, FooResp>("clientStreaming", this._transport, method, opt);
    }
    chat(options?: RpcOptions): DuplexStreamingCall<FooParam, FooResp> {
        return stackIntercept<FooParam, FooResp>("duplex", this._transport, method, opt);
    }
}
`

// httpClientSnippet is grpcClientSnippet in HTTP form
// httpClientSnippet 是 grpcClientSnippet 改写成 HTTP 形态后的样子
const httpClientSnippet = `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";
import { executeGrpcToHttp, executeGrpcToHttpStream, executeGrpcToHttpWebSocket } from '@yylego/grpc-to-http';
import type { GrpcToHttpPromise, GrpcToHttpStream, GrpcToHttpWebSocket } from '@yylego/grpc-to-http';
export class FooClient implements IFooClient {
    send(input: FooParam, options?: RpcOptions): GrpcToHttpPromise<FooParam, FooResp> {
        return executeGrpcToHttp<FooParam, FooResp>("unary", this._transport, method, opt, input);
    }
    watch(input: FooParam, options?: RpcOptions): GrpcToHttpStream<FooParam, FooResp> {
        return executeGrpcToHttpStream<FooParam, FooResp>(method, opt, input);
    }
    upload(options?: RpcOptions): GrpcToHttpWebSocket<FooParam, FooResp> {
        return executeGrpcToHttpWebSocket<FooParam, FooResp>(method, opt);
    }
    chat(options?: RpcOptions): GrpcToHttpWebSocket<FooParam, FooResp> {
        return executeGrpcToHttpWebSocket<FooParam, FooResp>(method, opt);
    }
}
`

// TestRewriteCodeAddsImportOnRepeatRun covers a client rewritten once with a unary method alone,
// then given a server-streaming method: the repeat run must import the stream symbols too.
//
// 覆盖：一个只有一元方法时改写过的客户端，后来加了服务端流式方法，再跑一次要把流式的符号也导入进来。
func TestRewriteCodeAddsImportOnRepeatRun(t *testing.T) {
	const halfDone = `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";
import { executeGrpcToHttp } from '@yylego/grpc-to-http';
import type { GrpcToHttpPromise } from '@yylego/grpc-to-http';
export class FooClient implements IFooClient {
    send(input: FooParam, options?: RpcOptions): GrpcToHttpPromise<FooParam, FooResp> {
        return executeGrpcToHttp<FooParam, FooResp>("unary", this._transport, method, opt, input);
    }
    watch(input: FooParam, options?: RpcOptions): ServerStreamingCall<FooParam, FooResp> {
        return stackIntercept<FooParam, FooResp>("serverStreaming", this._transport, method, opt, input);
    }
}
`
	const want = `import type { RpcOptions } from "@protobuf-ts/runtime-rpc";
import { executeGrpcToHttp, executeGrpcToHttpStream } from '@yylego/grpc-to-http';
import type { GrpcToHttpPromise, GrpcToHttpStream } from '@yylego/grpc-to-http';
export class FooClient implements IFooClient {
    send(input: FooParam, options?: RpcOptions): GrpcToHttpPromise<FooParam, FooResp> {
        return executeGrpcToHttp<FooParam, FooResp>("unary", this._transport, method, opt, input);
    }
    watch(input: FooParam, options?: RpcOptions): GrpcToHttpStream<FooParam, FooResp> {
        return executeGrpcToHttpStream<FooParam, FooResp>(method, opt, input);
    }
}
`
	out, err := rewriteCode(halfDone)
	require.NoError(t, err)
	require.Equal(t, want, out)
}

func TestRewriteCode(t *testing.T) {
	out, err := rewriteCode(grpcClientSnippet)
	require.NoError(t, err)
	require.Equal(t, httpClientSnippet, out)

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
// 守护防御逻辑：有 gRPC 调用、却没有 RpcOptions 地标导入的代码，调用会被替换、导入却插不进去，
// 此时必须报错，而不是产出"用了却没 import"的坏 TS。
func TestRewriteCodeMissingLandmarkErrors(t *testing.T) {
	const noLandmark = `export function foo() {
    return stackIntercept<FooParam, FooResp>("unary", this._transport, method, opt, input);
}
`
	_, err := rewriteCode(noLandmark)
	require.Error(t, err)
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
