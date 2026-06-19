package tgbot

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zixu5u/3xv/v3/internal/config"
	"github.com/zixu5u/3xv/v3/internal/logger"
	"github.com/zixu5u/3xv/v3/internal/util/common"
	"github.com/zixu5u/3xv/v3/internal/web/service"
)

// getServerAndInboundsStatus 获取服务器和所有启用节点的状态信息
// 返回格式化的状态消息，仅显示启用的节点
func (t *Tgbot) getServerAndInboundsStatus() string {
	var info strings.Builder

	// 获取服务器状态
	if cachedStatus, found := t.getCachedStatus(); found {
		t.lastStatus = cachedStatus
	} else {
		t.lastStatus = t.serverService.GetStatus(t.lastStatus)
		t.setCachedStatus(t.lastStatus)
	}

	// 第一部分：服务器基本信息
	info.WriteString("💻主机名称:" + hostname + "\r\n")
	info.WriteString("♻️系统类型:" + t.lastStatus.Os + "\r\n")
	info.WriteString("🚀系统架构:" + t.lastStatus.Arch + "\r\n")

	// 系统负载
	info.WriteString(fmt.Sprintf("🚥系统负载:%.2f,%.2f,%.2f\r\n",
		t.lastStatus.Loads[0], t.lastStatus.Loads[1], t.lastStatus.Loads[2]))

	// 运行时间
	upDays := t.lastStatus.Uptime / 86400
	upHours := (t.lastStatus.Uptime % 86400) / 3600
	upMins := (t.lastStatus.Uptime % 3600) / 60
	info.WriteString(fmt.Sprintf("⏰运行时间:%d天 %d小时 %d分钟\r\n",
		upDays, upHours, upMins))

	// Xray版本和状态
	info.WriteString("✨xray版本:" + fmt.Sprint(t.lastStatus.Xray.Version) + "\r\n")
	xrayStatus := "❌停止"
	if t.lastStatus.Xray.State == 1 { // Running = 1
		xrayStatus = "✅运行中"
	}
	info.WriteString("✅xray状态:" + xrayStatus + "\r\n")

	// 获取公网IP
	ipv4, ipv6 := t.getServerIPs()
	if ipv4 != "" {
		info.WriteString("📣IP地址:" + ipv4 + "\r\n")
	}
	if ipv6 != "" {
		info.WriteString("📍IPv6地址:" + ipv6 + "\r\n")
	}

	// 面板版本
	info.WriteString("🍪面板版本:" + config.GetVersion() + "\r\n")
	info.WriteString("\r\n")

	// 第二部分：所有启用的节点状态
	inbounds, err := t.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("GetAllInbounds failed:", err)
		info.WriteString("❌获取节点信息失败\r\n")
		return info.String()
	}

	// 统计启用的节点
	enabledCount := 0
	for _, inbound := range inbounds {
		if inbound.Enable {
			enabledCount++
		}
	}

	if enabledCount == 0 {
		info.WriteString("⚠️ 没有启用的节点\r\n")
		return info.String()
	}

	info.WriteString(fmt.Sprintf("📊 共 %d 个节点启用\r\n\r\n", enabledCount))

	// 逐个显示启用的节点
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue // 跳过禁用的节点
		}

		info.WriteString("🆔节点名称:" + inbound.Remark + "\r\n")
		info.WriteString("🔗节点类型:" + string(inbound.Protocol) + "\r\n")
		info.WriteString("🎯节点端口:" + strconv.Itoa(inbound.Port) + "\r\n")

		// 流量信息
		info.WriteString("⏫上行流量↑:" + common.FormatTraffic(inbound.Up) + "\r\n")
		info.WriteString("⏬下行流量↓:" + common.FormatTraffic(inbound.Down) + "\r\n")
		info.WriteString("📊整体流量:" + common.FormatTraffic(inbound.Up+inbound.Down) + "\r\n")

		// 总流量限制
		if inbound.Total > 0 {
			info.WriteString("❄️流量限制:" + common.FormatTraffic(inbound.Total) + "\r\n")
		} else {
			info.WriteString("❄️流量限制:♾️无限\r\n")
		}

		// 过期时间
		if inbound.ExpiryTime == 0 {
			info.WriteString("⏰到期时间:♾️\r\n")
		} else {
			expireTime := time.Unix((inbound.ExpiryTime / 1000), 0).Format("2006-01-02 15:04:05")
			info.WriteString("⏰到期时间:" + expireTime + "\r\n")
		}

		info.WriteString("\r\n")
	}

	return info.String()
}

// getServerIPs 获取服务器的IPv4和IPv6地址
func (t *Tgbot) getServerIPs() (string, string) {
	var ipv4, ipv6 string

	netInterfaces, err := net.Interfaces()
	if err != nil {
		logger.Error("net.Interfaces failed:", err)
		return "", ""
	}

	for _, ni := range netInterfaces {
		if (ni.Flags & net.FlagUp) != 0 {
			addrs, _ := ni.Addrs()

			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						if ipv4 == "" {
							ipv4 = ipnet.IP.String()
						}
					} else if ipnet.IP.To16() != nil && !ipnet.IP.IsLinkLocalUnicast() {
						if ipv6 == "" {
							ipv6 = ipnet.IP.String()
						}
					}
				}
			}
		}
	}

	return ipv4, ipv6
}

// handleStatusCommand 处理status按钮/命令
func (t *Tgbot) handleStatusCommand(chatId int64, callbackQuery *telego.CallbackQuery) {
	msg := t.getServerAndInboundsStatus()
	t.SendMsgToTgbot(chatId, msg)

	if callbackQuery != nil {
		t.sendCallbackAnswerTgBot(callbackQuery.ID, "✅ 状态已发送")
	}
}
