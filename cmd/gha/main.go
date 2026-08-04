package main

import (
	"fmt"
	"os"

	"github.com/zhongtait/gh-account/cmd"
	"github.com/zhongtait/gh-account/internal/terminal"
)

// main 是 gha 的可执行入口，负责统一输出命令错误并设置退出码。
func main() {
	if err := cmd.Execute(); err != nil {
		terminal.Error(os.Stderr, "%v", err)
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
}
