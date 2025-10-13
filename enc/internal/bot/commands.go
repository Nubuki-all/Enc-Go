package bot

import (
	"fmt"
	"strings"

	"github.com/gotd/td/tg"

	"enc/config"
)

var Commands string

func GetBotCommands() string {
	cmdSuffix := config.Conf.CmdSuffix
	lines := []string{
		fmt.Sprintf("start%s - check if bot is awake and get usage.", cmdSuffix),
		fmt.Sprintf("restart%s -  restart bot", cmdSuffix),
		fmt.Sprintf("update%s - update bot", cmdSuffix),
		fmt.Sprintf("nuke%s - ☢️ nuke bot", cmdSuffix),
		fmt.Sprintf("bash%s - /bash + command", cmdSuffix),
		fmt.Sprintf("eval%s - evaluate code", cmdSuffix),
		fmt.Sprintf("pause%s - prevent bot from encoding", cmdSuffix),
		fmt.Sprintf("peval%s - same as eval but with pyrogram", cmdSuffix),
		"ping - ping!",
		fmt.Sprintf("permit%s - add a temporary user", cmdSuffix),
		fmt.Sprintf("unpermit%s - removes a temporary user", cmdSuffix),
		fmt.Sprintf("add%s - add video to queue", cmdSuffix),
		fmt.Sprintf("l%s - add link to queue", cmdSuffix),
		fmt.Sprintf("ql%s - add torrent link to queue", cmdSuffix),
		fmt.Sprintf("s%s - select files from torrent to encode", cmdSuffix),
		fmt.Sprintf("queue%s - list queue", cmdSuffix),
		fmt.Sprintf("editq%s - edit queue", cmdSuffix),
		fmt.Sprintf("batch%s - preview batches", cmdSuffix),
		fmt.Sprintf("list%s - list all files in a link/torrent", cmdSuffix),
		fmt.Sprintf("listv%s - list all videos in a link (yt-dlp)", cmdSuffix),
		fmt.Sprintf("forward%s - manually forward a message to fchannel", cmdSuffix),
		fmt.Sprintf("v%s - turn v2,3,4… on (with message) or off", cmdSuffix),
		fmt.Sprintf("download%s - download a file or link to bot", cmdSuffix),
		fmt.Sprintf("upload%s - upload from a local directory or link", cmdSuffix),
		fmt.Sprintf("rename%s - rename a video file/link", cmdSuffix),
		fmt.Sprintf("m%s - get the media info of a replied file/link", cmdSuffix),
		fmt.Sprintf("mux%s - remux a file", cmdSuffix),
		fmt.Sprintf("get%s - get current ffmpeg code", cmdSuffix),
		fmt.Sprintf("set%s - set custom ffmpeg code", cmdSuffix),
		fmt.Sprintf("reset%s - reset default ffmpeg code", cmdSuffix),
		fmt.Sprintf("mset%s - set, reset, disable mux_args", cmdSuffix),
		fmt.Sprintf("mget%s - view current mux_args", cmdSuffix),
		fmt.Sprintf("filter%s - filter & stuff", cmdSuffix),
		fmt.Sprintf("vfilter%s - view filter", cmdSuffix),
		fmt.Sprintf("mtag%s - auto tag a file if matched", cmdSuffix),
		fmt.Sprintf("etag%s - view/modify auto tags", cmdSuffix),
		fmt.Sprintf("xtag%s - like mtag but only tags captions and matches release group names.", cmdSuffix),
		fmt.Sprintf("extag%s - view or delete xtags", cmdSuffix),
		fmt.Sprintf("groupenc%s - allow encoding in group toggle", cmdSuffix),
		fmt.Sprintf("delfilter%s - delete filter", cmdSuffix),
		fmt.Sprintf("airing%s - get anime airing info", cmdSuffix),
		fmt.Sprintf("anime%s - get anime info", cmdSuffix),
		fmt.Sprintf("name%s - quick filter with anime_title", cmdSuffix),
		fmt.Sprintf("vname%s - get list of name filter", cmdSuffix),
		fmt.Sprintf("delname%s - delete name filter", cmdSuffix),
		fmt.Sprintf("setrename%s - set custom_rename format", cmdSuffix),
		fmt.Sprintf("rss%s - edit, delete & subscribe rss feeds", cmdSuffix),
		fmt.Sprintf("status%s - 🆕 get bot's status", cmdSuffix),
		fmt.Sprintf("showthumb%s - 🖼️ show current thumbnail", cmdSuffix),
		fmt.Sprintf("parse%s - toggle parsing with captions or anilist", cmdSuffix),
		fmt.Sprintf("groupenc%s - turn off/on encoding in groups", cmdSuffix),
		fmt.Sprintf("cancelall%s - ❌ clear cached downloads & queued files", cmdSuffix),
		fmt.Sprintf("clear%s - clear queued files", cmdSuffix),
		fmt.Sprintf("logs%s - get bot logs", cmdSuffix),
		fmt.Sprintf("help%s - same as start", cmdSuffix),
	}

	Commands = strings.Join(lines, "\n")
	return Commands
}

func ParseCommands(cmds string) []tg.BotCommand {
	lines := strings.Split(cmds, "\n")
	commands := make([]tg.BotCommand, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "-", 2)
		if len(parts) < 2 {
			continue
		}

		command := strings.TrimSpace(parts[0])
		description := strings.TrimSpace(parts[1])

		// Remove leading "/" from command if present
		command = strings.TrimPrefix(command, "/")

		if command != "" && description != "" {
			commands = append(commands, tg.BotCommand{
				Command:     command,
				Description: description,
			})
		}
	}

	return commands
}
