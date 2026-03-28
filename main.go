package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"llm-local-proxy/config"
	"llm-local-proxy/provider"
	"llm-local-proxy/proxy"
)

func main() {
	var configFile string
	var debug bool
	flag.StringVar(&configFile, "config", "config.json", "配置文件路径")
	flag.BoolVar(&debug, "debug", false, "启用调试模式")
	flag.Parse()

	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		fmt.Println("请创建 config.json，参考 config.example.json")
		os.Exit(1)
	}

	// CLI flag overrides config
	if debug {
		cfg.Debug = true
	}

	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		fmt.Printf("❌ 初始化 provider 失败: %v\n", err)
		os.Exit(1)
	}

	handler := proxy.NewHandler(registry)

	fmt.Printf("🚀 LLM Proxy 已就绪: http://127.0.0.1%s\n", cfg.Listen)
	for _, p := range cfg.Providers {
		models := p.Models
		if len(models) == 0 {
			models = []string{"(none)"}
		}
		fmt.Printf("  📡 %s [%s] → %s  models: %v\n", p.Name, p.Type, p.BaseURL, models)
	}
	if cfg.Debug {
		fmt.Println("🔧 调试模式已启用")
	}

	if err := http.ListenAndServe(cfg.Listen, handler); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
	}
}
