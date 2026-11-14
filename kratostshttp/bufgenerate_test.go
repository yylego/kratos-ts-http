package kratostshttp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildBufArgs pins the argument sequence: the subcommand leads, the given
// arguments pass through untouched, and -o comes last.
//
// 钉住参数顺序：子命令打头，调用方自己的参数原样透传，-o 收尾。
func TestBuildBufArgs(t *testing.T) {
	args := buildBufArgs("/tmp/dest", []string{"--template", "buf.gen.ts.yaml", "api"})
	require.Equal(t, []string{
		"generate", "--template", "buf.gen.ts.yaml", "api", "-o", "/tmp/dest",
	}, args)
}

// TestBuildBufArgsWithoutOutPath pins the documented meaning of passing "": no -o is added,
// which leaves the destination to the plugin's own out: inside the template.
//
// 钉住传 "" 的约定含义：完全不加 -o，把落点交回模板里插件自己的 out: 决定。
// 这条容易在改动中被无意打破 —— 一旦顺手加上 -o ""，产物就会落到当前目录。
func TestBuildBufArgsWithoutOutPath(t *testing.T) {
	args := buildBufArgs("", []string{"--template", "buf.gen.ts.yaml"})
	require.Equal(t, []string{"generate", "--template", "buf.gen.ts.yaml"}, args)
}
