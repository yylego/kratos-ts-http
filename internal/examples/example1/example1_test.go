package example1

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yylego/kratos-ts-http/kratostshttp"
	"github.com/yylego/osexec/osexectest"
	"github.com/yylego/rese"
	"github.com/yylego/runpath"
)

// TestBufGenerate drives route ② end to end: it runs buf from Go, which runs the plugin,
// which runs protoc-gen-ts. The destination goes in as an argument, the template keeps
// `out: .`, so ONE place decides where output lands.
// 真跑一遍路线②：从 Go 里驱动 buf，buf 拉起插件，插件转手给 protoc-gen-ts。
// 落点作为参数传进去，模板里保持 `out: .` —— 只有一个地方决定产物落在哪。
//
// It writes to the SAME place on each run, on purpose. A fresh temp DIR would hand each run a
// clean slate and hide anything that piles up: nested output, stale files a rename left
// behind. Writing in place also lets `git status` show the drift, which beats a test message.
// 它每次都写到【同一个地方】，这是刻意的。临时目录每次都给一块白纸，
// 反而把越积越多的东西藏起来：嵌套的产出、改名留下的旧文件。就地覆盖还让 `git status`
// 把漂移直接摆出来，比一句测试报错更好用。
//
// The tools live outside Go, so it steps aside when one goes missing, instead of painting
// CI red for something that is not a defect.
// 这些工具在 Go 之外，缺了就跳过，而不是让 CI 因为一件不是缺陷的事变红。
func TestBufGenerate(t *testing.T) {
	for _, name := range []string{"buf", "protoc-gen-kratos-ts-http", "protoc-gen-ts"} {
		osexectest.SkipIfCommandNotFound(t, name)
	}

	require.NoError(t, kratostshttp.BufGenerate(runpath.PARENT.Path(), "web/src/rpc", []string{
		"--template", "buf.gen.ts.yaml",
		"--include-imports",
	}))
}

// TestGoldenCheck demands the client matches golden.ts. When the test above has just run, this
// checks what it produced. When the tools are missing and it stepped aside, this checks
// the two committed files, so something of use happens in both cases.
// 要求客户端跟 golden.ts 一致。上面那条刚跑过时，这里检查的就是它的产物；
// 工具缺失、上面跳过时，这里检查的仍是已提交的那一对 —— 两种情况下都有事可做。
func TestGoldenCheck(t *testing.T) {
	want := rese.A1(os.ReadFile(runpath.PARENT.Join("golden.ts")))
	got := rese.A1(os.ReadFile(runpath.PARENT.Join("web", "src", "rpc", "greeter", "v1", "greeter.client.ts")))
	require.Equal(t, string(want), string(got))
}
