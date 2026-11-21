package kratostshttp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yylego/osexec/osexectest"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// TestRunTsPlugin drives one complete exchange as buf does: a CodeGeneratorRequest goes in
// on stdin, protoc-gen-ts itself runs, and the response on stdout holds the rewritten client.
// It steps aside when protoc-gen-ts is missing, as the example test does.
//
// 照 buf 的方式跑一次完整交互：CodeGeneratorRequest 从 stdin 进，真的 protoc-gen-ts 跑一遍，
// stdout 上的响应里是改写后的客户端。没装 protoc-gen-ts 时跳过，跟示例测试一样。
func TestRunTsPlugin(t *testing.T) {
	osexectest.SkipIfCommandNotFound(t, tsPluginName)

	// the smallest proto: one service with one unary method
	// 最小的 proto：一个服务、一个一元方法
	fooFile := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("foo/v1/foo.proto"),
		Package: proto.String("foo.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("FooParam"), Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("name"), Number: proto.Int32(1), JsonName: proto.String("name"),
				Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}}},
			{Name: proto.String("FooResp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("FooService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name: proto.String("Send"), InputType: proto.String(".foo.v1.FooParam"), OutputType: proto.String(".foo.v1.FooResp"),
			}},
		}},
	}
	request, err := proto.Marshal(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"foo/v1/foo.proto"},
		Parameter:      proto.String("long_type_string"),
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fooFile},
	})
	require.NoError(t, err)

	var stdout bytes.Buffer
	stats, err := RunTsPlugin(bytes.NewReader(request), &stdout)
	require.NoError(t, err)
	require.Equal(t, GenStats{FileCount: 2, EditCount: 1}, stats)

	// the result is the protoc-gen-ts output with the rewrite rules applied, the client file alone touched
	// 结果就是 protoc-gen-ts 的产物套上改写规则，只动了客户端文件
	srcResponse, err := delegateToTsPlugin(request)
	require.NoError(t, err)
	srcFiles := decodeResponse(t, srcResponse)
	httpClient, err := rewriteCode(srcFiles["foo/v1/foo.client.ts"])
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"foo/v1/foo.ts":        srcFiles["foo/v1/foo.ts"],
		"foo/v1/foo.client.ts": httpClient,
	}, decodeResponse(t, stdout.Bytes()))
}

// messageSnippet is a message type file, which must pass through untouched
// messageSnippet 是消息类型文件，必须原样透传、一个字都不能改
//
// The client fixture is shared with the rest of the tests in this package (grpcClientSnippet),
// so both routes remain pinned to the same input.
// 客户端样本跟本包其他测试共用（grpcClientSnippet），这样两条路线钉住的是同一份输入。
const messageSnippet = `import { MessageType } from "@protobuf-ts/runtime";
class FooParam$Type extends MessageType<FooParam> {}
`

// buildResponse builds a CodeGeneratorResponse carrying the given files
// buildResponse 构造一个带指定文件的 CodeGeneratorResponse
func buildResponse(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var response pluginpb.CodeGeneratorResponse
	for name, content := range files {
		response.File = append(response.File, &pluginpb.CodeGeneratorResponse_File{
			Name: proto.String(name), Content: proto.String(content),
		})
	}
	data, err := proto.Marshal(&response)
	require.NoError(t, err)
	return data
}

// decodeResponse parses a response and indexes its files with the file name
// decodeResponse 解析响应并按文件名建索引
func decodeResponse(t *testing.T, data []byte) map[string]string {
	t.Helper()
	var response pluginpb.CodeGeneratorResponse
	require.NoError(t, proto.Unmarshal(data, &response))
	files := map[string]string{}
	for _, file := range response.File {
		files[file.GetName()] = file.GetContent()
	}
	return files
}

// TestRewriteResponse is the core contract: client files get converted,
// message files must come back unchanged, down to the byte.
//
// 核心契约：客户端文件被改写，消息类型文件必须逐字节原样返回。
// 这条比"改写得对不对"更要紧：消息文件里是 @protobuf-ts/runtime 的序列化实现，
// 改动一个字都可能让前端运行时出错。
func TestRewriteResponse(t *testing.T) {
	srcResponse := buildResponse(t, map[string]string{
		"foo/foo.client.ts": grpcClientSnippet,
		"foo/foo.ts":        messageSnippet,
	})

	newResponse, stats, err := rewriteResponse(srcResponse)
	require.NoError(t, err)
	require.Equal(t, 2, stats.FileCount)
	require.Equal(t, 1, stats.EditCount, "just .client.ts files should be rewritten")

	files := decodeResponse(t, newResponse)
	require.Equal(t, messageSnippet, files["foo/foo.ts"], "message type files must come back untouched")
	require.Equal(t, httpClientSnippet, files["foo/foo.client.ts"])
}

// TestRewriteResponseIsIdempotent guards the case where someone runs the plugin
// across converted output, chaining it onto the old two step flow when migrating.
//
// 幂等性：万一有人把插件跑在已经转换过的产物上（比如迁移期间新旧流程并存），
// 不能把代码改坏。底层 rewriteCode 承诺幂等，这里把它钉住。
func TestRewriteResponseIsIdempotent(t *testing.T) {
	once, _, err := rewriteResponse(buildResponse(t, map[string]string{"a.client.ts": grpcClientSnippet}))
	require.NoError(t, err)
	twice, _, err := rewriteResponse(once)
	require.NoError(t, err)
	require.Equal(t, decodeResponse(t, once)["a.client.ts"], decodeResponse(t, twice)["a.client.ts"])
}

// TestRewriteResponsePropagatesPluginError makes sure the underlying plugin's
// own error is surfaced instead of being swallowed.
//
// 底层插件自己报错时必须原样带出来 —— 吞掉的话，用户只看到我们这层的报错，
// 而真正的原因（比如 proto 写错）被掩盖，排查方向随之偏离。
func TestRewriteResponsePropagatesPluginError(t *testing.T) {
	var response pluginpb.CodeGeneratorResponse
	response.Error = proto.String("missing semicolon at line 3")
	data, err := proto.Marshal(&response)
	require.NoError(t, err)

	_, _, err = rewriteResponse(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing semicolon at line 3")
}

// TestRewriteResponseHandlesNilContent covers files carrying no content
// 有些文件可以只带名字不带内容（protoc 协议允许，用于 insertion point 之类），
// 不能因为它就中断。
func TestRewriteResponseHandlesNilContent(t *testing.T) {
	var response pluginpb.CodeGeneratorResponse
	response.File = append(response.File, &pluginpb.CodeGeneratorResponse_File{Name: proto.String("x.client.ts")})
	data, err := proto.Marshal(&response)
	require.NoError(t, err)

	newResponse, stats, err := rewriteResponse(data)
	require.NoError(t, err)
	require.Equal(t, 1, stats.FileCount)
	require.Equal(t, 0, stats.EditCount, "a file with no content must not count as rewritten")
	require.Equal(t, map[string]string{"x.client.ts": ""}, decodeResponse(t, newResponse))
}

// TestRewriteResponseRejectsGarbage makes sure a corrupt response fails at once
// 收到坏数据要明确报错，别静默返回空结果 —— 那会让调用方以为"生成成功但没文件"。
func TestRewriteResponseRejectsGarbage(t *testing.T) {
	_, _, err := rewriteResponse([]byte("this is not protobuf"))
	require.Error(t, err)
}
