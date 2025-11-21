package main

import (
	"context"
	"fmt"
	"log"
	"nofx/api"
	"nofx/config"
	"nofx/manager"
	"nofx/mcp"
	"nofx/news"
	"nofx/pool"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║    🏆 AI模型交易竞赛系统 - Qwen vs DeepSeek               ║")
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

	// 初始化新闻服务
	var newsSvc *news.Service
	if cfg.NewsRSSURL != "" || cfg.NewsWebsocketURL != "" {
		log.Printf("📰 初始化新闻服务...")
		newsOpts := news.Options{
			WebsocketURL: cfg.NewsWebsocketURL,
			RSSURL:       cfg.NewsRSSURL,
			StorageDir:   cfg.NewsStorageDir,
		}

		// 解析时间配置
		if cfg.NewsMaxAge != "" {
			if d, err := time.ParseDuration(cfg.NewsMaxAge); err == nil {
				newsOpts.MaxAge = d
			}
		}
		if cfg.NewsPersistCooldown != "" {
			if d, err := time.ParseDuration(cfg.NewsPersistCooldown); err == nil {
				newsOpts.PersistCooldown = d
			}
		}
		if cfg.NewsReconnectDelay != "" {
			if d, err := time.ParseDuration(cfg.NewsReconnectDelay); err == nil {
				newsOpts.ReconnectDelay = d
			}
		}
		if cfg.NewsPingInterval != "" {
			if d, err := time.ParseDuration(cfg.NewsPingInterval); err == nil {
				newsOpts.PingInterval = d
			}
		}

		// 如果配置了AI，设置新闻摘要器（使用第一个启用的trader的AI配置）
		var summarizer news.Summarizer
		for _, trader := range cfg.Traders {
			if !trader.Enabled {
				continue
			}
			mcpClient := mcp.New()
			if trader.AIModel == "custom" {
				if trader.CustomAPIURL != "" && trader.CustomAPIKey != "" {
					mcpClient.SetCustomAPI(trader.CustomAPIURL, trader.CustomAPIKey, trader.CustomModelName)
					summarizer = news.NewMCPSummarizer(mcpClient)
					log.Printf("📰 使用自定义AI (%s) 进行新闻摘要", trader.Name)
					break
				}
			} else if trader.AIModel == "qwen" {
				if trader.QwenKey != "" {
					mcpClient.SetQwenAPIKey(trader.QwenKey, "")
					summarizer = news.NewMCPSummarizer(mcpClient)
					log.Printf("📰 使用Qwen (%s) 进行新闻摘要", trader.Name)
					break
				}
			} else if trader.AIModel == "deepseek" {
				if trader.DeepSeekKey != "" {
					mcpClient.SetDeepSeekAPIKey(trader.DeepSeekKey)
					summarizer = news.NewMCPSummarizer(mcpClient)
					log.Printf("📰 使用DeepSeek (%s) 进行新闻摘要", trader.Name)
					break
				}
			}
		}
		newsOpts.Summarizer = summarizer

		var err error
		newsSvc, err = news.NewService(newsOpts)
		if err != nil {
			log.Printf("⚠️  新闻服务初始化失败: %v，将不使用新闻功能", err)
		} else {
			news.SetDefaultService(newsSvc)
			log.Printf("✓ 新闻服务初始化成功")

			// 启动新闻服务后台任务
			ctx := context.Background()
			go func() {
				newsSvc.Run(ctx)
			}()
			log.Printf("✓ 新闻服务后台任务已启动")
		}
	} else {
		log.Printf("⏭️  未配置新闻服务，跳过初始化")
	}

	// 创建TraderManager
	traderManager := manager.NewTraderManager()

	// 添加所有启用的trader
	enabledCount := 0
	for i, traderCfg := range cfg.Traders {
		// 跳过未启用的trader
		if !traderCfg.Enabled {
			log.Printf("⏭️  [%d/%d] 跳过未启用的 %s", i+1, len(cfg.Traders), traderCfg.Name)
			continue
		}

		enabledCount++
		log.Printf("📦 [%d/%d] 初始化 %s (%s模型)...",
			i+1, len(cfg.Traders), traderCfg.Name, strings.ToUpper(traderCfg.AIModel))

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
			traderCfg.Name, strings.ToUpper(traderCfg.AIModel), traderCfg.InitialBalance)
	}

	fmt.Println()
	fmt.Println("🤖 AI全权决策模式:")
	fmt.Printf("  • AI将自主决定每笔交易的杠杆倍数（山寨币最高%d倍，BTC/ETH最高%d倍）\n",
		cfg.Leverage.AltcoinLeverage, cfg.Leverage.BTCETHLeverage)
	fmt.Println("  • AI将自主决定每笔交易的仓位大小")
	fmt.Println("  • AI将自主设置止损和止盈价格")
	fmt.Println("  • AI将基于市场数据、技术指标、账户状态做出全面分析")
	fmt.Println()
	fmt.Println("⚠️  风险提示: AI自动交易有风险，建议小额资金测试！")
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
	fmt.Println("👋 感谢使用AI交易竞赛系统！")
}
