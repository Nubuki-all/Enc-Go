package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/shirou/gopsutil/v3/load"
	"enc"
	"enc/internal/bot"
	"enc/utils" // <-- replace with your actual utils import path
)

// StatusHandler gathers system + bot statistics using gopsutil and sends them
// as a formatted message. It assumes utils.UserIsAllowed and messaging helpers exist.
func StatusHandler(ctx context.Context, update *ext.Update, msg *tele.Message) error {
	// permission check
	if !utils.UserIsAllowed(msg.UserID) {
		return nil
	}

	version := enc.Version

	// Bot uptime from utils (assumes utils.BotStartTime() exists)
	uptime := time.Since(bot.BotStartTime).Round(time.Second).String()

	// Host info
	hostInfo, _ := host.Info()
	hostname := ""
	if hostInfo != nil {
		hostname = hostInfo.Hostname
	}

	// Load averages (1m,5m,15m)
	loadAvg := ""
	if l, err := load.Avg(); err == nil {
		loadAvg = fmt.Sprintf("%.2f, %.2f, %.2f", l.Load1, l.Load5, l.Load15)
	}

	// CPU counts & percent
	logicalCores, _ := cpu.Counts(true)
	physicalCores, _ := cpu.Counts(false)
	cpuPerc := float64(0)
	if p, err := cpu.Percent(time.Second, false); err == nil && len(p) > 0 {
		cpuPerc = p[0]
	}

	// Memory
	vm, _ := mem.VirtualMemory()
	swapS, _ := mem.SwapMemory()

	// Disk (root)
	du, _ := disk.Usage("/")

	// Network IO (aggregate)
	var netSent uint64
	var netRecv uint64
	if ioCounters, err := net.IOCounters(false); err == nil && len(ioCounters) > 0 {
		netSent = ioCounters[0].BytesSent
		netRecv = ioCounters[0].BytesRecv
	}

	// Process info for current process (optional: to show RSS/cpu of bot process)
	selfPID := os.Getpid()
	var procCPU float64
	var procMem uint64
	if p, err := process.NewProcess(int32(selfPID)); err == nil {
		if c, err := p.CPUPercent(); err == nil {
			procCPU = c
		}
		if m, err := p.MemoryInfo(); err == nil {
			procMem = m.RSS
		}
	}

	// Format human-readable sizes (uses your helper)
	hTotal := utils.GetReadableFileSize(du.Total)
	hUsed := utils.GetReadableFileSize(du.Used)
	hFree := utils.GetReadableFileSize(du.Free)
	hSent := utils.GetReadableFileSize(netSent)
	hRecv := utils.GetReadableFileSize(netRecv)
	hMemTotal := utils.GetReadableFileSize(vm.Total)
	hMemUsed := utils.GetReadableFileSize(vm.Used)
	hMemAvail := utils.GetReadableFileSize(vm.Available)
	hSwapTotal := utils.GetReadableFileSize(swapS.Total)

	// Build message
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Version:** `%s`\n", version))
	b.WriteString("\n")
	if hostname != "" {
		b.WriteString(fmt.Sprintf("**Host:** `%s`\n", hostname))
	}
	if hostInfo != nil && hostInfo.Platform != "" {
		b.WriteString(fmt.Sprintf("**OS:** `%s %s %s`\n", hostInfo.Platform, hostInfo.PlatformVersion, runtime.GOARCH))
	} else {
		b.WriteString(fmt.Sprintf("**Platform:** `%s`\n", runtime.GOOS))
	}
	b.WriteString(fmt.Sprintf("**Bot Uptime:** `%s`\n", uptime))
	if loadAvg != "" {
		b.WriteString(fmt.Sprintf("**Load (1,5,15):** `%s`\n", loadAvg))
	}
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("**CPU:** `%0.2f%%` (phys: %d, logical: %d)\n", cpuPerc, physicalCores, logicalCores))
	b.WriteString(fmt.Sprintf("**Memory:** `%s` total • `%s` used • `%s` available • `%d%%`\n", hMemTotal, hMemUsed, hMemAvail, vm.UsedPercent))
	b.WriteString(fmt.Sprintf("**Swap:** `%s` • `%d%%`\n", hSwapTotal, swapS.UsedPercent))
	b.WriteString(fmt.Sprintf("**Disk (/):** `%s` total • `%s` used • `%s` free • `%0.2f%%`\n", hTotal, hUsed, hFree, du.UsedPercent))
	b.WriteString(fmt.Sprintf("**Network:** `↑ %s` • `↓ %s`\n", hSent, hRecv))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("**Process (self):** CPU `%0.2f%%` • RSS `%s`\n", procCPU, utils.GetReadableFileSize(procMem)))
	b.WriteString(fmt.Sprintf("**Runtime:** Go `%s` • Arch `%s`\n", runtime.Version(), runtime.GOARCH))

	// Send reply
	_, err := msg.ReplyText(client.String())
	return err
}
