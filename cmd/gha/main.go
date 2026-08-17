package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zhongtait/gh-account/cmd"
	"github.com/zhongtait/gh-account/internal/terminal"
	"github.com/zhongtait/gh-account/internal/utils"
)

var (
	executeCLI            = cmd.Execute
	errorOutput io.Writer = os.Stderr
	exitProcess           = os.Exit
)

// main 是 gha 的可执行入口，负责统一输出命令错误并设置退出码。
func main() {
	if err := executeCLI(); err != nil {
		// 脱敏错误消息中的敏感信息
		sanitized := utils.RedactSensitiveData(err.Error())
		terminal.Error(errorOutput, "%s", sanitized)
		fmt.Fprintln(errorOutput)
		exitProcess(1)
	}
}
