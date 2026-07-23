package kratostshttp

import (
	"os"
	"os/exec"
	"strings"

	"github.com/yylego/erero"
	"github.com/yylego/zaplog"
	"go.uber.org/zap"
)

const bufCommandName = "buf"

// BufGenerate runs one `buf generate`
// 执行一次 `buf generate`
//
//	workRoot: where buf runs, buf resolves each of its relative paths against this
//	outPath:  the `-o` value; pass "" to leave -o off and let the template's own `out:` decide
//	bufArgs:  buf's own flags, not ours, so these pass through as-is instead of being modelled
//	workRoot: 在哪儿执行 buf，buf 的相对路径都相对它解析
//	outPath:  对应 `-o`；传 "" 就不加 -o，落点交给模板里插件自己的 `out:` 决定
//	bufArgs:  这些 flag 是 buf 的、不是我们的，所以原样透传，不逐个建模
//
// ⚠️ This function rewrites nothing itself. The rewrite happens two processes down, and the
// sole link between them is a string inside a YAML file that no build step can see:
// ⚠️ 本函数自己不做任何替换。替换发生在往下两层的进程里，二者仅由 YAML 里的一行字符串相连，
// 编译器看不见：
//
//	BufGenerate ─exec→ buf ─(模板里写着 local: protoc-gen-kratos-ts-http)→ protoc-gen-kratos-ts-http
//	                                                                    └─ 替换在这里
//
// The two directions are opposite: at compile time cmd/protoc-gen-kratos-ts-http imports this
// package; at run time this package spawns that command. Reading the Go alone shows just half
// the picture; the second half lives in the YAML.
// 两个方向是相反的：编译期 cmd/protoc-gen-kratos-ts-http 导入本包，运行期本包启动那个命令。
// 只读 Go 代码只能看到其中一半，另一半在 YAML 里。
//
// ⛔ So the template decides the output shape, not this code. Point the template at
// protoc-gen-ts and this function returns nil just the same, while the output stays in gRPC
// form: success here means "buf ran", not "the aim was met". The split is deliberate: the
// template is where the plugin gets chosen, and each project writes a template regardless, so
// choosing it here too would add nothing.
// ⛔ 所以产物形态由模板决定，不由这里决定。把模板指向 protoc-gen-ts，本调用一样返回 nil，
// 产物却停留在 gRPC 形态：这里的"成功"只表示"buf 运行完成"，不表示"目标达成"。这是有意的分工——
// 模板本就是选择插件的地方，每个项目都要写模板，在这里再选一次并无益处。
//
// 💡 `--config` / `--template` accept a file path, and accept the config content itself just
// the same, so a config built at run time can go straight into bufArgs without a temp file.
// ⛔ But buf tells the two apart from how the string ends (.yaml/.yml/.json), not from the
// presence of such a file on disk, so content ending in ".yaml" -- even inside a trailing
// comment -- is opened as a filename and fails with something like `open version: v2`, an
// error distant from the true cause. Watch the last line when passing content.
// 💡 `--config` / `--template` 既接受文件路径，也接受配置内容本身，运行时拼出的配置可以直接
// 传入 bufArgs，无需临时文件。⛔ 但 buf 区分二者，看的是字符串结尾（.yaml/.yml/.json），
// 而非磁盘上是否真有这个文件；所以内容若以 ".yaml" 结尾（哪怕只在行尾注释里），就会被当作
// 文件名去打开，报出 `open version: v2` 这类离真正原因很远的错误。传内容时留意最后一行。
func BufGenerate(workRoot string, outPath string, bufArgs []string) error {
	if strings.TrimSpace(workRoot) == "" {
		return erero.New("work-root is required but blank")
	}
	if len(bufArgs) == 0 {
		return erero.New("buf-args is required but has no items")
	}

	// Check before exec, so the error can name what is missing
	// 执行前先检查，好让错误信息指出缺少什么
	if _, err := exec.LookPath(bufCommandName); err != nil {
		return erero.Errorf("command %s not found, Kratos v3 projects ship it, please install: go install github.com/bufbuild/buf/cmd/buf@latest", bufCommandName)
	}

	args := buildBufArgs(outPath, bufArgs)

	zaplog.LOG.Info("run-buf-generate",
		zap.String("work-root", workRoot),
		zap.String("out-path", outPath),
		zap.Strings("args", args))

	cmd := exec.Command(bufCommandName, args...)
	cmd.Dir = workRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// buf printed the true reason to stderr above; here we just note which run failed
		// buf 的真实原因已经打到上面的 stderr，这里只补一句是哪次调用失败的
		return erero.Wrapf(err, "buf generate failed (work-root=%s out-path=%s)", workRoot, outPath)
	}
	return nil
}

// buildBufArgs assembles the buf argument list, a blank outPath adds no `-o`
// 拼出交给 buf 的参数列表，outPath 为空就完全不加 `-o`
func buildBufArgs(outPath string, bufArgs []string) []string {
	args := append([]string{"generate"}, bufArgs...)
	if outPath != "" {
		args = append(args, "-o", outPath)
	}
	return args
}
