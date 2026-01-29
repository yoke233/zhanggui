package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	defaultEnvPrefix = "TASKCTL"
)

type Options struct {
	ConfigPath string
	EnvPrefix  string
}

func Load(configPath string) error {
	return LoadWithOptions(Options{
		ConfigPath: configPath,
		EnvPrefix:  defaultEnvPrefix,
	})
}

func LoadWithOptions(opts Options) error {
	prefix := strings.TrimSpace(opts.EnvPrefix)
	if prefix == "" {
		prefix = defaultEnvPrefix
	}

	viper.SetEnvPrefix(prefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// taskctl defaults
	viper.SetDefault("base-dir", "fs/taskctl")
	viper.SetDefault("sandbox-mode", "docker")
	viper.SetDefault("sandbox-image", "alpine:3.20")
	viper.SetDefault("sandbox-network", "none")
	viper.SetDefault("timeout-seconds", 900)
	viper.SetDefault("log-level", "info")

	// zhanggui server defaults（不影响 taskctl；不同二进制单独进程运行）
	viper.SetDefault("http-addr", "127.0.0.1")
	viper.SetDefault("http-port", 8020)
	viper.SetDefault("runs-dir", "fs/runs")
	viper.SetDefault("protocol", "agui.v0")

	if opts.ConfigPath != "" {
		viper.SetConfigFile(opts.ConfigPath)
		if err := viper.ReadInConfig(); err != nil {
			return err
		}
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// prefix 不同则默认配置目录也不同，避免互相干扰。
	switch strings.ToUpper(prefix) {
	case "ZHANGGUI":
		viper.AddConfigPath(filepath.Join(home, ".zhanggui"))
	default:
		viper.AddConfigPath(filepath.Join(home, ".taskctl"))
	}
	viper.AddConfigPath(".")

	_ = viper.ReadInConfig() // 没有配置文件也不报错
	return nil
}
