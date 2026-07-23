package kratostshttp

import (
	"bytes"
	"io"
	"os/exec"
	"strings"

	"github.com/yylego/erero"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// This file is the protoc-plugin route behind the protoc-gen-kratos-ts-http command.
// The rewrite happens before the response is returned, so what buf writes to disk is the
// finished code, and the output DIR is a result instead of an intermediate.
// 本文件是 protoc 插件这条路线，protoc-gen-kratos-ts-http 命令的实现。
// 改写在响应返回之前完成，所以 buf 落盘的已经是最终代码，输出目录是结果，而非中间产物。
//
// The public API takes just RunTsPlugin. The two helpers return intermediate results: one
// hands back unrewritten gRPC-form clients, the second hands back raw protoc wire bytes, so
// the public API keeps them out.
// 只有 RunTsPlugin 是导出的。另外两个返回的都是中间结果：一个是尚未改写的 gRPC 形态客户端，
// 一个是 protoc 的原始字节，都不宜作为对外接口。
//
// ⛔ This path must not log. zaplog writes to stdout unless told otherwise, and here stdout
// carries the CodeGeneratorResponse, so a single log line corrupts the wire format; buf
// then reports an unreadable protobuf, an error distant from the true cause. Logging is fine
// elsewhere in this package (see bufgenerate.go); on this path, report progress on stderr.
// ⛔ 这条路径不能打日志。zaplog 默认写 stdout，而这里的 stdout 载着 CodeGeneratorResponse，
// 一行日志就会破坏协议，buf 随后报出一句 protobuf 解析失败——这个错误离真正的原因很远。本包
// 其余地方打日志没问题（见 bufgenerate.go），唯独这条路不行，需要报进度请走 stderr。

// tsPluginName does the proto→TS translation itself, it ships in npm @protobuf-ts/plugin
// tsPluginName 是真正把 proto 翻译成 TS 的那个插件，随 npm 包 @protobuf-ts/plugin 发布
const tsPluginName = "protoc-gen-ts"

// GenStats is what one run reports back
// GenStats 是跑完一趟之后交回的结果
type GenStats struct {
	// FileCount covers each file the upstream plugin handed back, untouched ones included
	// FileCount 是上游插件一共交回多少个文件，含没动过的
	FileCount int
	// EditCount covers the subset rewritten into HTTP form
	// EditCount 是其中有多少个被改写成了 HTTP 形态
	EditCount int
}

// RunTsPlugin performs one complete protoc-plugin exchange, stdin request to stdout response
// 完成一次完整的 protoc 插件交互，从 stdin 的请求到 stdout 的响应
//
// The calling code reports the stats, this function stays silent, see the stdout warning above.
// 统计结果交给调用方去报告，本函数自己保持沉默，原因见上面关于 stdout 的警告。
func RunTsPlugin(stdin io.Reader, stdout io.Writer) (GenStats, error) {
	// Raw bytes, no parsing: the opt flags (ts_nocheck etc) ride inside, and passing them
	// through untouched means we do not need to understand them
	// 读原始字节、不解析：opt 参数（ts_nocheck 之类）就在里面，原样透传就不用去理解它们
	srcRequest, err := io.ReadAll(stdin)
	if err != nil {
		return GenStats{}, erero.Wrap(err, "read CodeGeneratorRequest failed")
	}

	srcResponse, err := delegateToTsPlugin(srcRequest)
	if err != nil {
		return GenStats{}, erero.Wro(err)
	}

	newResponse, stats, err := rewriteResponse(srcResponse)
	if err != nil {
		return GenStats{}, erero.Wro(err)
	}

	if _, err := stdout.Write(newResponse); err != nil {
		return GenStats{}, erero.Wrap(err, "write CodeGeneratorResponse failed")
	}
	return stats, nil
}

// delegateToTsPlugin passes the request to protoc-gen-ts and returns its raw response.
// delegateToTsPlugin 把请求交给 protoc-gen-ts，取回它的原始响应。
//
// Delegating costs less than generating TS ourselves: just the client files (*.client.ts,
// ~57 lines) need changing, while the message files (*.ts, ~250 lines each and more numerous)
// contain @protobuf-ts/runtime's serialization internals. Reimplementing those would mean
// matching that runtime byte to byte across each release, and drift there breaks the frontend
// at run time.
// 为什么转手、而不自己生成 TS：只有客户端文件（*.client.ts，约 57 行）需要改动，而消息类型
// 文件（*.ts，每个约 250 行、数量更多）包含 @protobuf-ts/runtime 的序列化实现。自己重写就得
// 逐字节跟随它的每次发布，一旦不一致，前端运行时便会出错。
func delegateToTsPlugin(srcRequest []byte) ([]byte, error) {
	// Check before exec, so the error can name what is missing
	// 执行前先检查，好让错误信息指出缺少什么
	if _, err := exec.LookPath(tsPluginName); err != nil {
		return nil, erero.Errorf("plugin %s not found, please install it: npm install -g @protobuf-ts/plugin", tsPluginName)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(tsPluginName)
	cmd.Stdin = bytes.NewReader(srcRequest)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Surface the plugin's own stderr; otherwise the error is just "exit status 1"
		// 带出插件自己的 stderr，否则只有一句 exit status 1，难以定位
		return nil, erero.Wrapf(err, "run %s failed, its output: %s", tsPluginName, stderr.String())
	}
	return stdout.Bytes(), nil
}

// rewriteResponse rewrites the gRPC TS clients inside a protoc response into HTTP clients;
// message-type files pass through untouched.
// rewriteResponse 把 protoc 响应里的 gRPC TS 客户端改写为 HTTP 客户端，消息类型文件原样透传。
func rewriteResponse(srcResponse []byte) ([]byte, GenStats, error) {
	var response pluginpb.CodeGeneratorResponse
	if err := proto.Unmarshal(srcResponse, &response); err != nil {
		return nil, GenStats{}, erero.Wrapf(err, "parse CodeGeneratorResponse from %s failed", tsPluginName)
	}
	// Pass the plugin's own error through: swallowing it would hide the true cause, such as a
	// mistake in the proto, and send the investigation in the wrong direction
	// 原样传出底层插件的报错：吞掉会掩盖真正的原因（比如 proto 写错了），把排查引向错误的方向
	if response.Error != nil && *response.Error != "" {
		return nil, GenStats{}, erero.Errorf("%s reported an error: %s", tsPluginName, *response.Error)
	}

	stats := GenStats{FileCount: len(response.File)}
	for _, file := range response.File {
		// A file can have a name with no content, the protoc wire format permits it
		// 文件可以只带名字不带内容，protoc 协议是允许的
		if !strings.HasSuffix(file.GetName(), clientFileSuffix) || file.Content == nil {
			continue
		}
		newCode, err := rewriteCode(file.GetContent())
		if err != nil {
			return nil, GenStats{}, erero.Wrapf(err, "rewrite %s failed", file.GetName())
		}
		file.Content = proto.String(newCode)
		stats.EditCount++
	}

	newResponse, err := proto.Marshal(&response)
	if err != nil {
		return nil, GenStats{}, erero.Wrap(err, "serialize CodeGeneratorResponse failed")
	}
	return newResponse, stats, nil
}
