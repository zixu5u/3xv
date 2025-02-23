package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"x-ui/config"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/util/common"
	"x-ui/web/global"
	"x-ui/xray"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/robfig/cron/v3"
)

type Tgbot struct {
	bot            *tgbotapi.BotAPI
	SettingService *SettingService
	InboundService *InboundService
	UserService    *UserService
	xrayService    *XrayService
	cron           *cron.Cron
	stopping       bool
	cronId         cron.EntryID
}

var (
	tgbot     *Tgbot
	tgbotOnce sync.Once
)

func GetTgbot() *Tgbot {
	tgbotOnce.Do(func() {
		tgbot = &Tgbot{
			SettingService: &SettingService{},
			InboundService: &InboundService{},
			UserService:    &UserService{},
			xrayService:    &XrayService{},
			cron:           cron.New(),
			stopping:       false,
		}
	})
	return tgbot
}

func (t *Tgbot) Start() {
	if !t.SettingService.GetTgbotEnabled() {
		logger.Info("Telegram bot is disabled in settings.")
		return
	}

	// 初始化 Bot
	token := t.SettingService.GetTgBotToken()
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		logger.Error("Failed to initialize Telegram bot:", err)
		return
	}
	t.bot = bot
	logger.Info("Telegram bot initialized successfully.")

	// 设置命令菜单
	t.setCommandMenu()

	// 启动定时任务
	go t.startCron()

	// 获取更新
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := t.bot.GetUpdatesChan(u)

	// 处理消息和回调
	for update := range updates {
		if t.stopping {
			break
		}
		if update.Message != nil {
			go t.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go t.handleCallback(update.CallbackQuery)
		}
	}
}

func (t *Tgbot) Stop() {
	t.stopping = true
	if t.bot != nil {
		t.bot.StopReceivingUpdates()
	}
	if t.cron != nil {
		t.cron.Stop()
	}
	logger.Info("Telegram bot stopped.")
}

// 设置命令菜单
func (t *Tgbot) setCommandMenu() {
	commands := []tgbotapi.BotCommand{
		{Command: "/start", Description: "Start the bot"},
		{Command: "/menu", Description: "Show available options"},
	}
	config := tgbotapi.NewSetMyCommands(commands...)
	_, err := t.bot.Request(config)
	if err != nil {
		logger.Error("Failed to set command menu:", err)
		return
	}
	logger.Info("Telegram bot command menu set successfully.")
}

// 处理消息
func (t *Tgbot) handleMessage(msg *tgbotapi.Message) {
	if !t.checkAdmin(msg.Chat.ID) {
		t.sendMsg(msg.Chat.ID, "You are not authorized to use this bot.")
		return
	}

	switch msg.Text {
	case "/start":
		t.sendMsg(msg.Chat.ID, "Welcome to 3X-UI Bot! Use /menu to see options.")
	case "/menu":
		t.showMenu(msg.Chat.ID)
	default:
		t.sendMsg(msg.Chat.ID, "Unknown command. Use /menu to see options.")
	}
}

// 显示菜单（内联键盘）
func (t *Tgbot) showMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Select an option:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Functions", "functions"),
			tgbotapi.NewInlineKeyboardButtonData("Status", "status"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Restart", "restart"),
			tgbotapi.NewInlineKeyboardButtonData("Clear All", "clearall"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Help", "help"),
		),
	)
	_, err := t.bot.Send(msg)
	if err != nil {
		logger.Error("Failed to send menu:", err)
	}
}

// 处理内联键盘回调
func (t *Tgbot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	switch callback.Data {
	case "functions":
		t.sendMsg(chatID, "Available functions:\n- Traffic stats\n- User management\n(to be expanded)")
	case "status":
		t.sendStatus(chatID)
	case "restart":
		t.restartServer(chatID)
	case "clearall":
		t.clearAll(chatID)
	case "help":
		t.sendMsg(chatID, "Help:\n/menu - Show options\nContact admin for more info.")
	default:
		t.sendMsg(chatID, "Unknown option.")
	}

	// 确认回调已处理
	t.bot.Request(tgbotapi.NewCallback(callback.ID, ""))
}

// 发送系统状态
func (t *Tgbot) sendStatus(chatID int64) {
	inbounds, err := t.InboundService.GetAllInbounds()
	if err != nil {
		t.sendMsg(chatID, "Failed to get status: "+err.Error())
		return
	}
	statusMsg := "System Status:\n"
	for _, inbound := range inbounds {
		statusMsg += fmt.Sprintf("Inbound %s: %s\n", inbound.Tag, common.FormatTraffic(inbound.Total))
	}
	cpuPercent := t.getCPUUsage()
	statusMsg += fmt.Sprintf("CPU Usage: %.2f%%\n", cpuPercent)
	t.sendMsg(chatID, statusMsg)
}

// 重启服务器
func (t *Tgbot) restartServer(chatID int64) {
	t.sendMsg(chatID, "Restarting 3X-UI...")
	err := global.GetWebServer().Stop()
	if err != nil {
		t.sendMsg(chatID, "Failed to stop server: "+err.Error())
		return
	}
	err = global.GetWebServer().Start()
	if err != nil {
		t.sendMsg(chatID, "Failed to start server: "+err.Error())
		return
	}
	t.sendMsg(chatID, "3X-UI restarted successfully.")
}

// 清理所有数据（示例）
func (t *Tgbot) clearAll(chatID int64) {
	t.sendMsg(chatID, "Clearing all data...")
	// 示例：清理流量统计
	err := t.InboundService.ClearTraffic()
	if err != nil {
		t.sendMsg(chatID, "Failed to clear data: "+err.Error())
		return
	}
	t.sendMsg(chatID, "All data cleared successfully.")
}

// 发送消息辅助函数
func (t *Tgbot) sendMsg(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := t.bot.Send(msg)
	if err != nil {
		logger.Error("Failed to send message:", err)
	}
}

// 检查管理员权限
func (t *Tgbot) checkAdmin(chatID int64) bool {
	chatIDs := t.SettingService.GetTgBotChatId()
	for _, id := range strings.Split(chatIDs, ",") {
		if id == "" {
			continue
		}
		if cid, err := strconv.ParseInt(id, 10, 64); err == nil && cid == chatID {
			return true
		}
	}
	return false
}

// 定时任务
func (t *Tgbot) startCron() {
	runtime := t.SettingService.GetTgbotRuntime()
	if runtime == "" {
		runtime = "0 0 8 * * *" // 默认每天早上 8 点
	}

	id, err := t.cron.AddFunc(runtime, t.sendDailyReport)
	if err != nil {
		logger.Error("Failed to add cron job:", err)
		return
	}
	t.cronId = id
	t.cron.Start()
	logger.Info("Telegram bot cron started with schedule:", runtime)
}

// 发送每日报告
func (t *Tgbot) sendDailyReport() {
	chatIDs := t.SettingService.GetTgBotChatId()
	inbounds, err := t.InboundService.GetAllInbounds()
	if err != nil {
		logger.Error("Failed to get inbounds for report:", err)
		return
	}

	report := "Daily Traffic Report:\n"
	for _, inbound := range inbounds {
		report += fmt.Sprintf("%s: %s\n", inbound.Tag, common.FormatTraffic(inbound.Total))
	}
	for _, chatIDStr := range strings.Split(chatIDs, ",") {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			continue
		}
		t.sendMsg(chatID, report)
	}
}

// 获取 CPU 使用率（示例）
func (t *Tgbot) getCPUUsage() float64 {
	// 这里应调用系统监控逻辑，示例返回固定值
	return 45.5
}

// 通知登录事件
func (t *Tgbot) NotifyLogin(username string, ip string) {
	if !t.SettingService.GetTgbotEnabled() || t.bot == nil {
		return
	}
	msg := fmt.Sprintf("User %s logged in from IP %s at %s", username, ip, time.Now().Format(time.RFC1123))
	chatIDs := t.SettingService.GetTgBotChatId()
	for _, chatIDStr := range strings.Split(chatIDs, ",") {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			continue
		}
		t.sendMsg(chatID, msg)
	}
}

// 通知流量上限
func (t *Tgbot) NotifyTrafficLimit(inbound *model.Inbound) {
	if !t.SettingService.GetTgbotEnabled() || t.bot == nil {
		return
	}
	msg := fmt.Sprintf("Inbound %s has reached traffic limit: %s", inbound.Tag, common.FormatTraffic(inbound.Total))
	chatIDs := t.SettingService.GetTgBotChatId()
	for _, chatIDStr := range strings.Split(chatIDs, ",") {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			continue
		}
		t.sendMsg(chatID, msg)
	}
}

// 通知到期日期
func (t *Tgbot) NotifyExpiration(inbound *model.Inbound, daysLeft int) {
	if !t.SettingService.GetTgbotEnabled() || t.bot == nil {
		return
	}
	msg := fmt.Sprintf("Inbound %s will expire in %d days.", inbound.Tag, daysLeft)
	chatIDs := t.SettingService.GetTgBotChatId()
	for _, chatIDStr := range strings.Split(chatIDs, ",") {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			continue
		}
		t.sendMsg(chatID, msg)
	}
}

// 通知 CPU 负载
func (t *Tgbot) NotifyCPULoad(usage float64) {
	if !t.SettingService.GetTgbotEnabled() || t.bot == nil {
		return
	}
	msg := fmt.Sprintf("CPU load has exceeded threshold: %.2f%%", usage)
	chatIDs := t.SettingService.GetTgBotChatId()
	for _, chatIDStr := range strings.Split(chatIDs, ",") {
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			continue
		}
		t.sendMsg(chatID, msg)
	}
}
