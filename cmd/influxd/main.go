// Command influxd is the InfluxDB server.
package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/influxdata/influxdb/cmd"
	"github.com/influxdata/influxdb/cmd/influxd/backup"
	"github.com/influxdata/influxdb/cmd/influxd/help"
	"github.com/influxdata/influxdb/cmd/influxd/restore"
	"github.com/influxdata/influxdb/cmd/influxd/run"
)

// These variables are populated via the Go linker.
var (
	//记录版本信息
	version string
	commit  string
	branch  string
)

func init() {
	// If commit, branch, or build time are not set, make that clear.
	// 如果没有设置版本信息，设置为默认值: unknow
	if version == "" {
		version = "unknown"
	}
	if commit == "" {
		commit = "unknown"
	}
	if branch == "" {
		branch = "unknown"
	}
}

func main() {
	// 设置随机数, 给rand函数随机种子
	rand.Seed(time.Now().UnixNano())

	// 主逻辑， 初始化Main结构， 前台执行Run操作， Run方法中监听系统调用信号
	m := NewMain()
	if err := m.Run(os.Args[1:]...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Main represents the program execution.
type Main struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewMain return a new instance of Main.
func NewMain() *Main {
	return &Main{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Run determines and runs the command specified by the CLI args.
func (m *Main) Run(args ...string) error {
	// 应该使用flag参数优化一下， 这样看起来就容易理解
	name, args := cmd.ParseCommandName(args)

	// Extract name from args.
	switch name {
	case "", "run": // server后台执行
		cmd := run.NewCommand()

		// Tell the server the build details.
		cmd.Version = version
		cmd.Commit = commit
		cmd.Branch = branch

		if err := cmd.Run(args...); err != nil { //主逻辑是cmd.Run方法
			return fmt.Errorf("run: %s", err)
		}

		// 系统signal监听
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
		cmd.Logger.Info("Listening for signals")

		// Block until one of the signals above is received
		<-signalCh
		cmd.Logger.Info("Signal received, initializing clean shutdown...")
		// 异步执行Close方法
		go cmd.Close()

		// Block again until another signal is received, a shutdown timeout elapses,
		// or the Command is gracefully closed
		cmd.Logger.Info("Waiting for clean shutdown...")
		select {
		case <-signalCh: //如果用户第二次方法系统信号就直接hard shutdown
			cmd.Logger.Info("Second signal received, initializing hard shutdown")
		case <-time.After(time.Second * 30): // shutdown超时
			cmd.Logger.Info("Time limit reached, initializing hard shutdown")
		case <-cmd.Closed: //shutdown正常结束
			cmd.Logger.Info("Server shutdown completed")
		}

		// goodbye.

	case "backup":
		name := backup.NewCommand()
		if err := name.Run(args...); err != nil {
			return fmt.Errorf("backup: %s", err)
		}
	case "restore":
		name := restore.NewCommand()
		if err := name.Run(args...); err != nil {
			return fmt.Errorf("restore: %s", err)
		}
	case "config":
		if err := run.NewPrintConfigCommand().Run(args...); err != nil {
			return fmt.Errorf("config: %s", err)
		}
	case "version": // 输出版本信息
		if err := NewVersionCommand().Run(args...); err != nil {
			return fmt.Errorf("version: %s", err)
		}
	case "help":
		if err := help.NewCommand().Run(args...); err != nil {
			return fmt.Errorf("help: %s", err)
		}
	default:
		return fmt.Errorf(`unknown command "%s"`+"\n"+`Run 'influxd help' for usage`+"\n\n", name)
	}

	return nil
}

// VersionCommand represents the command executed by "influxd version".
type VersionCommand struct {
	Stdout io.Writer
	Stderr io.Writer
}

// NewVersionCommand return a new instance of VersionCommand.
func NewVersionCommand() *VersionCommand {
	return &VersionCommand{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Run prints the current version and commit info.
func (cmd *VersionCommand) Run(args ...string) error {
	// Parse flags in case -h is specified.
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(cmd.Stderr, versionUsage) }
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Print version info.
	fmt.Fprintf(cmd.Stdout, "InfluxDB v%s (git: %s %s)\n", version, branch, commit)

	return nil
}

var versionUsage = `Displays the InfluxDB version, build branch and git commit hash.

Usage: influxd version
`
