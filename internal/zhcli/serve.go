package zhcli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yoke233/zhanggui/internal/a2a"
	"github.com/yoke233/zhanggui/internal/agui"
	"github.com/yoke233/zhanggui/internal/logging"
)

const (
	flagHTTPAddr  = "http-addr"
	flagHTTPPort  = "http-port"
	flagRunsDir   = "runs-dir"
	flagBasePath  = "base-path"
	flagLogLevel  = "log-level"
	flagProtocol  = "protocol"
	flagReadmeMsg = "print-endpoints"
	flagA2ABase   = "a2a-base-path"
)

func NewServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "启动 AG-UI 对接服务（SSE + tool_result + interrupt/resume demo）",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := strings.TrimSpace(viper.GetString(flagHTTPAddr))
			port := viper.GetInt(flagHTTPPort)
			runsDir := strings.TrimSpace(viper.GetString(flagRunsDir))
			basePath := strings.TrimSpace(viper.GetString(flagBasePath))
			protocol := strings.TrimSpace(viper.GetString(flagProtocol))
			a2aBase := strings.TrimSpace(viper.GetString(flagA2ABase))
			if addr == "" {
				addr = "127.0.0.1"
			}
			if port <= 0 {
				port = 8020
			}
			if runsDir == "" {
				runsDir = "fs/runs"
			}
			if basePath == "" {
				basePath = "/agui"
			}
			if !strings.HasPrefix(basePath, "/") {
				basePath = "/" + basePath
			}
			basePath = strings.TrimSuffix(basePath, "/")
			if protocol == "" {
				protocol = "agui.v0"
			}
			if a2aBase == "" {
				a2aBase = "/a2a"
			}
			if !strings.HasPrefix(a2aBase, "/") {
				a2aBase = "/" + a2aBase
			}
			a2aBase = strings.TrimSuffix(a2aBase, "/")

			logPath := filepath.Join(runsDir, "_server", "logs", "server.log")
			logger, closeLogger, err := logging.NewLogger(logging.Options{
				Stdout:  os.Stderr,
				LogPath: logPath,
				Level:   logging.ParseLevel(viper.GetString(flagLogLevel)),
			})
			if err != nil {
				return err
			}
			defer func() { _ = closeLogger() }()

			coreStore := a2a.NewStore()

			h, err := agui.NewHandler(agui.Options{
				RunsDir:   runsDir,
				BasePath:  basePath,
				Protocol:  protocol,
				Logger:    logger,
				CoreStore: coreStore,
			})
			if err != nil {
				return err
			}

			mux := http.NewServeMux()
			h.Register(mux)

			a2aHandler, err := a2a.NewHandler(a2a.Options{
				BasePath: a2aBase,
				Logger:   logger,
				Store:    coreStore,
			})
			if err != nil {
				return err
			}
			a2aHandler.Register(mux)

			httpAddr := fmt.Sprintf("%s:%d", addr, port)
			logger.Info("server start", "addr", httpAddr, "base_path", basePath, "runs_dir", runsDir, "protocol", protocol)

			if viper.GetBool(flagReadmeMsg) {
				fmt.Fprintln(cmd.OutOrStdout(), "endpoints:")
				fmt.Fprintln(cmd.OutOrStdout(), "  GET  /healthz")
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/run (SSE)\n", basePath)
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/tool_result\n", basePath)
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/action (A2UI client action)\n", basePath)
				fmt.Fprintln(cmd.OutOrStdout(), "  GET  /.well-known/agent-card.json")
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/message:send\n", a2aBase)
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/message:stream (SSE)\n", a2aBase)
				fmt.Fprintf(cmd.OutOrStdout(), "  GET  %s/tasks\n", a2aBase)
				fmt.Fprintf(cmd.OutOrStdout(), "  GET  %s/tasks/{id}\n", a2aBase)
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/tasks/{id}:cancel\n", a2aBase)
				fmt.Fprintf(cmd.OutOrStdout(), "  POST %s/tasks/{id}:subscribe (SSE)\n", a2aBase)
				fmt.Fprintf(cmd.OutOrStdout(), "  GET  %s/extendedAgentCard\n", a2aBase)
			}

			return http.ListenAndServe(httpAddr, mux)
		},
	}

	cmd.Flags().String(flagHTTPAddr, "127.0.0.1", "HTTP 监听地址")
	cmd.Flags().Int(flagHTTPPort, 8020, "HTTP 监听端口")
	cmd.Flags().String(flagRunsDir, "fs/runs", "运行目录（fs/runs；不入 git）")
	cmd.Flags().String(flagBasePath, "/agui", "AG-UI base path（用于预留修改路径）")
	cmd.Flags().String(flagProtocol, "agui.v0", "对外协议名（预留升级/转换；本阶段仅 agui.v0）")
	cmd.Flags().String(flagLogLevel, "info", "日志级别：debug|info|warn|error")
	cmd.Flags().Bool(flagReadmeMsg, true, "启动时打印 endpoints 到 stdout")
	cmd.Flags().String(flagA2ABase, "/a2a", "A2A HTTP/REST base path")

	_ = viper.BindPFlags(cmd.Flags())

	return cmd
}
