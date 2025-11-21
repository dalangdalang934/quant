package main

import (
	"fmt"
	"log"
	"nofx/api"
	"nofx/config"
	"nofx/manager"
	"nofx/pool"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║            📈 量化交易执行与监控系统                      ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 加载配置文件
	configFile := "config.json"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	log.Printf("📋 加载配置文件: %s", configFile)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	log.Printf("✓ 配置加载成功，共%d个trader参赛", len(cfg.Traders))
	fmt.Println()

	// 设置默认主流币种列表
	pool.SetDefaultCoins(cfg.DefaultCoins)

	// 设置是否使用默认主流币种
	pool.SetUseDefaultCoins(cfg.UseDefaultCoins)
	if cfg.UseDefaultCoins {
		log.Printf("✓ 已启用默认主流币种列表（共%d个币种）: %v", len(cfg.DefaultCoins), cfg.DefaultCoins)
	}

	// 设置币种池API URL
	if cfg.CoinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(cfg.CoinPoolAPIURL)
		log.Printf("✓ 已配置AI500币种池API")
	}
	if cfg.OITopAPIURL != "" {
		pool.SetOITopAPI(cfg.OITopAPIURL)
		log.Printf("✓ 已配置OI Top API")
	}

	// （新闻摘要功能已停用）

	// 创建TraderManager
	traderManager := manager.NewTraderManager()

	// 仅保留第一个启用的 trader
	enabledCount := 0
	addedTrader := false
	for i, traderCfg := range cfg.Traders {
		// 跳过未启用的trader
		if !traderCfg.Enabled {
			log.Printf("⏭️  [%d/%d] 跳过未启用的 %s", i+1, len(cfg.Traders), traderCfg.Name)
			continue
		}
		if addedTrader {
			log.Printf("⏭️  [%d/%d] 系统仅运行一个交易员，已忽略 %s", i+1, len(cfg.Traders), traderCfg.Name)
			continue
		}

		enabledCount++
		log.Printf("📦 [%d/%d] 初始化 %s (%s 策略)...",
			i+1, len(cfg.Traders), traderCfg.Name, strings.ToUpper(traderCfg.Strategy))

		err := traderManager.AddTrader(
			traderCfg,
			cfg.CoinPoolAPIURL,
			cfg.MaxDailyLoss,
			cfg.MaxDrawdown,
			cfg.StopTradingMinutes,
			cfg.Leverage, // 传递杠杆配置
		)
		if err != nil {
			log.Fatalf("❌ 初始化trader失败: %v", err)
		}
		addedTrader = true
	}

	// 检查是否至少有一个启用的trader
	if enabledCount == 0 {
		log.Fatalf("❌ 没有启用的trader，请在config.json中设置至少一个trader的enabled=true")
	}

	fmt.Println()
	fmt.Println("🏁 竞赛参赛者:")
	for _, traderCfg := range cfg.Traders {
		// 只显示启用的trader
		if !traderCfg.Enabled {
			continue
		}
		fmt.Printf("  • %s (%s) - 初始资金: %.0f USDT\n",
			traderCfg.Name, strings.ToUpper(traderCfg.Strategy), traderCfg.InitialBalance)
	}

	fmt.Println()
	fmt.Println("📐 策略说明:")
	fmt.Printf("  • 系统会根据量化信号自动评估多空机会（山寨币最高%d倍，BTC/ETH最高%d倍）\n",
		cfg.Leverage.AltcoinLeverage, cfg.Leverage.BTCETHLeverage)
	fmt.Println("  • 仓位大小、止损止盈均由策略参数与账户情况动态计算")
	fmt.Println("  • 仅保留一个量化交易员，聚焦执行与复盘")
	fmt.Println()
	fmt.Println("⚠️  风险提示: 自动交易仍有市场风险，建议在小额资金上验证策略后再扩大资金规模")
	fmt.Println()
	fmt.Println("按 Ctrl+C 停止运行")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// 创建并启动API服务器
	apiServer := api.NewServer(traderManager, cfg.APIServerPort)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("❌ API服务器错误: %v", err)
		}
	}()

	// 设置优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 启动所有trader
	traderManager.StartAll()

	// 等待退出信号
	<-sigChan
	fmt.Println()
	fmt.Println()
	log.Println("📛 收到退出信号，正在停止所有trader...")
	traderManager.StopAll()

	fmt.Println()
	fmt.Println("👋 感谢使用量化交易监控系统！")
}
