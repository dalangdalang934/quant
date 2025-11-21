package main

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/logger"
	"nofx/trader"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使用方法: go run migrate-positions.go <trader_id>")
		fmt.Println("例如: go run migrate-positions.go binance_deepseek")
		os.Exit(1)
	}

	traderID := os.Args[1]
	log.Printf("🔄 开始迁移 %s 的仓位数据...", traderID)

	// 1. 读取现有的仓位历史文件
	oldFile := fmt.Sprintf("data/position_history/%s.json", traderID)
	data, err := os.ReadFile(oldFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("⚠️ 文件不存在: %s", oldFile)
			log.Println("没有需要迁移的数据")
			return
		}
		log.Fatalf("❌ 读取文件失败: %v", err)
	}

	// 2. 解析旧格式数据
	var oldHistory struct {
		Trades      []logger.TradeOutcome `json:"trades"`
		LastUpdated time.Time            `json:"last_updated"`
	}
	if err := json.Unmarshal(data, &oldHistory); err != nil {
		log.Fatalf("❌ 解析JSON失败: %v", err)
	}

	log.Printf("📊 找到 %d 条历史交易记录", len(oldHistory.Trades))

	// 3. 创建仓位追踪器
	tracker := trader.NewPositionTracker(traderID)
	
	// 4. 转换数据
	// 由于历史数据已经是平仓记录，我们需要模拟创建仓位
	migratedCount := 0
	
	for i, trade := range oldHistory.Trades {
		// 为每个交易生成一个仓位
		// 注意：这是一种简化的迁移，假设每个交易都是独立的完整仓位
		
		// 创建仓位
		position := tracker.CreatePosition(
			trade.Symbol,
			trade.Side,
			trade.OpenPrice,
			trade.OpenQuantity,
			trade.Leverage,
			fmt.Sprintf("migrated_%d", i),
		)
		
		// 设置仓位时间
		position.OpenTime = trade.OpenTime
		position.UpdatedAt = trade.CloseTime
		
		// 立即平仓（因为这些都是历史已平仓记录）
		err := tracker.ClosePosition(
			position.ID,
			trade.ClosePrice,
			trade.Quantity,
			trade.PnL,
			0, // 手续费信息可能不完整
			fmt.Sprintf("migrated_close_%d", i),
			trade.CloseNote,
		)
		
		if err != nil {
			log.Printf("⚠️ 迁移第 %d 条记录失败: %v", i+1, err)
			continue
		}
		
		migratedCount++
		if migratedCount%10 == 0 {
			log.Printf("  已迁移 %d/%d 条记录...", migratedCount, len(oldHistory.Trades))
		}
	}

	// 5. 备份原文件
	backupFile := fmt.Sprintf("data/position_history/%s_backup_%s.json", 
		traderID, time.Now().Format("20060102_150405"))
	if err := os.Rename(oldFile, backupFile); err != nil {
		log.Printf("⚠️ 备份原文件失败: %v", err)
	} else {
		log.Printf("✅ 原文件已备份到: %s", backupFile)
	}

	log.Printf("✅ 迁移完成！共迁移 %d 条记录", migratedCount)
	log.Printf("📁 新数据已保存到:")
	log.Printf("   - 活跃仓位: data/positions/%s_active.json", traderID)
	log.Printf("   - 历史仓位: data/positions/%s_history.json", traderID)
}
