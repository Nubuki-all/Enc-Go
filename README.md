# [ENC-Go]

[![Checks](https://github.com/Nubuki-all/Enc-Go/actions/workflows/GoLint.yml/badge.svg?branch=main)](https://github.com/Nubuki-all/Enc-Go/actions/workflows/GoLint.yml)

A Telegram bot that downloads videos from Telegram, direct links, torrents, Mega and yt-dlp sites, encodes them with your own FFmpeg (or HandBrakeCLI) commands, and uploads the results back to Telegram, with anime-aware naming, captions and thumbnails.

__Still in beta…__

## Features

**Encoding**
- Encode queue with persistent storage (MongoDB, or a local SQLite file when no database is set)
- Up to four encode commands per item (`FFMPEG`, `FFMPEG2`–`FFMPEG4`), run one after another or in parallel (`SEQUENTIAL_JOBS`, `MAX_SEQUENTIAL_JOBS`)
- Optional remux step after encoding (`MUX_ARGS`, `/mset`), with metadata placeholders and default audio/subtitle track selection
- Change encode commands at runtime (`/set`, `/get`, `/reset`) without restarting
- Pause encoding indefinitely or for a set time (`/pause`, `LOCK_ON_STARTUP`)
- Cancel the current encode, the whole queue, or everything at once (`/cancelall`)

**Sources**
- Telegram videos and documents, auto-queued from private chat if `AUTO_QUEUE_VID` is set
- Direct links via aria2, torrents and magnets via qBittorrent, Mega links, and any site yt-dlp supports
- Pick single files from torrents, folders and playlists, or queue the whole thing as a batch (`/select`, `/batch`)
- Download-only and upload-only commands for moving files without encoding (`/download`, `/upload`)

**Naming and captions**
- Parses anime release names with anitomy and looks titles up on AniList for names, captions, thumbnails and airing info
- Custom rename format (`/setrename`, `CUSTOM_RENAME`) and per-title rename filters (`/name`)
- Filters to strip or replace parts of file names and captions (`/filter`), plus automatic tags by file name or release group (`/mtag`, `/xtag`)
- Release name, caption decoration, channel link and version tags (`RELEASER`, `CAP_DECO`, `C_LINK`, `/v`)
- MediaInfo posted to Telegraph (`/m`)

**Channels and RSS**
- Forward finished encodes to a channel with banners, stickers and a live status message (`FCHANNEL`, `FCHANNEL_STAT`, `FSTICKER`)
- Log channel for job logs and errors (`LOG_CHANNEL`, `LOGS_IN_CHANNEL`)
- Dump leeched files to a channel (`DUMP_CHANNEL`, `DUMP_LEECH`)
- RSS monitor: subscribe to feeds and pass new items to any bot command, with include/exclude keyword filters, topics, pausing and rollback (`/rss`)

**Operations**
- Downloads its own static builds of FFmpeg, aria2, qBittorrent-nox, MediaInfo, anitomy and yt-dlp (with Deno) into `.bin/` on first start
- Self-update from GitHub releases (`/update`)
- Owner and temporary-user permissions (`OWNER`, `TEMP_USERS`, `/permit`), with developer commands for a separate `DEV` user
- Bot status with CPU, RAM, disk and uptime (`/status`), shell access (`/bash`, `/sh`), Go eval (`/eval`), logs (`/logs`) and a process view (`/top`)
- Flood-wait handling, auto-restart interval and optional pprof endpoint

## Deployment

The bot reads its settings from a `.env` file in the directory it runs from (or from environment variables). Copy [.env.sample](.env.sample) to `.env` and fill in at least the compulsory variables below.

**With Docker**
- Install Docker
- Clone the repository and add your `.env` next to the `Dockerfile`
- Run:
  - `docker build . -t enc`
  - `docker run enc`

The runtime image is Fedora. It needs glibc, because the yt-dlp and Deno builds the bot downloads are glibc binaries (they don't run on Alpine's musl), and it needs a shell with `bash` for `/bash`. For a smaller image, `debian:trixie-slim` with `ca-certificates` also works.

**Without Docker**
- Download the executable for your architecture (Linux amd64, arm64 or armv7) from releases and run it from a directory containing your `.env`

**From source**
- Install Go 1.26 or newer
- Clone the repository; the Go module is at the repository root
- Build and run:
  - `go build -o enc .`
  - `./enc`
- Run the tests with `go test ./...`

## Configuration

Don't put quotes around values in `.env`.

### Compulsory

Variable | Description
:--- | :---
`APP_ID` `API_HASH` | From [my.telegram.org](https://my.telegram.org)
`BOT_TOKEN` | From [@BotFather](https://t.me/botfather)
`OWNER` | User ids allowed full control of the bot, separated by spaces

### Encoding

Variable | Default | Description
:--- | :--- | :---
`FFMPEG` | an x265 CRF 27 command | Encode command. Use `{input}` and `{output}` for the file paths, e.g. `ffmpeg -i {input} -preset veryfast -c:v libx265 -crf 27 {output}`. HandBrakeCLI also works once installed
`FFMPEG2` `FFMPEG3` `FFMPEG4` | | Extra encode commands run for every item
`MUX_ARGS` | | FFmpeg arguments (no input/output) for a remux after encoding; maps, metadata and dispositions only
`SEQUENTIAL_JOBS` | `true` | Set to `false` to run the encode commands of one item in parallel
`MAX_SEQUENTIAL_JOBS` | `1` | Most encodes running at once when parallel, never more than CPU count / 4; `0` uses that limit
`LOCK_ON_STARTUP` | `false` | Start with encoding paused until `/pause off`
`ALLOW_ACTION` | `true` | Show the "playing a game" chat action while encoding

In `FFMPEG*` and `MUX_ARGS`, `This Episode` in a `-metadata` value is replaced with "Episode N of Title Season S" and `Fileinfo` with the parsed file name. In encode commands, `-disposition:a auto` and `-disposition:s auto` mark the English audio and subtitle tracks as default.

### Access

Variable | Default | Description
:--- | :--- | :---
`TEMP_USERS` | | Extra user ids allowed to use the bot, separated by spaces
`DEV` | | User id allowed the developer commands (`/bash`, `/eval`, `/sh`, `/logs`, `/top`) besides the owners
`NO_TEMP_PM` | `false` | Temporary users can only use the bot in groups, not in private chat
`CMD_SUFFIX` | | Only answer commands ending with this suffix (for running several bots in one group)

### Names, captions and uploads

Variable | Default | Description
:--- | :--- | :---
`USE_ANILIST` | `true` | Look titles up on AniList
`USE_CAPTION` | `true` | Treat a one-line caption on a sent video as its file name
`FILENAME_AS_CAPTION` | `false` | Use the file name as the caption
`CUSTOM_RENAME` | | Rename format; see `/setrename` for the variables
`RELEASER` | `A-M\|ANi-MiNE` | Release name tag; `short\|long` or `none`
`ENCODER` | | Encoder name or encode info for captions
`CAP_DECO` | `◉` | Decoration before caption lines
`C_LINK` | `@ANi_MiNE` | Channel link in captions
`THUMBNAIL` | | Telegraph link of the default thumbnail
`UPLOAD_AS_DOCUMENT` | `true` | Upload as documents rather than videos
`UPLOAD_VIDEO_AS_SPOILER` | `false` | Blur uploaded videos; needs `UPLOAD_AS_DOCUMENT=false`
`TELEGRAPH_API` | `https://api.telegra.ph` | Telegraph API used for MediaInfo pages
`TELEGRAPH_AUTHOR` | | `name\|url` for Telegraph pages (`auto` for either part)
`FINISHED_PROGRESS_STR` `UN_FINISHED_PROGRESS_STR` | random hearts, `🤍` | Progress bar characters
`PROGRESS_UPDATE_INTERVAL` | `10` | Seconds between progress updates (at least 2)

### Channels

Variable | Default | Description
:--- | :--- | :---
`FCHANNEL` | | Channel id where finished encodes are forwarded (bot must be admin)
`FCHANNEL_STAT` | | Message id in `FCHANNEL` to keep updated as a live status
`FSTICKER` | | Sticker sent after a release in `FCHANNEL`
`FCODEC` | | Codec and quality shown in `FCHANNEL` banners
`FBANNER` | `false` | Post banners more often in `FCHANNEL`
`NO_BANNER` | `false` | Don't post banners in `FCHANNEL`
`LOG_CHANNEL` | | Group or channel id for job logs (bot must be admin)
`LOGS_IN_CHANNEL` | `false` | Also send error logs to `LOG_CHANNEL`
`DUMP_LEECH` | `true` | Send leeched files to the bot's private chat and the dump channel
`DUMP_CHANNEL` | | Channel or group id for leeched files
`REPORT_FAILED` | `true` | Report failed downloads and encodes; set to `false` to choose with `REPORT_FAILED_DL` and `REPORT_FAILED_ENC`

### Downloads

Variable | Default | Description
:--- | :--- | :---
`AUTO_QUEUE_VID` | `false` | Queue videos sent to the bot in private chat automatically
`CACHE_DL` | `false` | Download the next queued item while the current one encodes
`DL_STUFF` | | Comma-separated links to download on startup
`NO_EXTERNAL_DL_SERVICE` | `false` | Don't start aria2 and qBittorrent
`PODI` | `true` | Keep a download paused while its file list is shown; `false` to disable
`ARIA2_PORT` | `6800` | aria2 RPC port
`QBIT_PORT` `QBIT_PORT2` | `8090` `9090` | qBittorrent ports (downloads, file-list lookups)
`QBIT_TIMEOUT` | `20` | Seconds to wait for torrent metadata; `0` waits forever
`ARIA2_DL_TIMEOUT` `QBIT_DL_TIMEOUT` `MEGA_DL_TIMEOUT` `YT_DL_TIMEOUT` | | Give up on a download after this many seconds
`YTDLP_TIMEOUT` | | Give up on fetching yt-dlp video info after this many seconds
`TG_UL_TIMEOUT` | | Give up on a Telegram upload after this many seconds

### RSS

Variable | Default | Description
:--- | :--- | :---
`RSS_CHAT` | | Chat new feed items are sent to; the RSS monitor stops if unset
`RSS_CHAT_TOPIC` | | Topic id in `RSS_CHAT`
`RSS_DELAY` | `60` | Seconds between feed checks
`RSS_DIRECT` | `true` | Pass feed items straight to the bot command instead of only posting them

### Storage and runtime

Variable | Default | Description
:--- | :--- | :---
`DATABASE_URL` | | MongoDB URL; without it a local SQLite database is used
`DBNAME` | `ENC-GO` | MongoDB database name
`LOCAL_DB_PATH` | `.encbot.db` | Path of the SQLite file
`DOWNLOAD_DIR` `ALT_DOWNLOAD_DIR` `ENCODE_DIR` `MUX_DIR` `MEDIAINFO_DIR` `DUMP_DIR` `TEMP_DIR` | `.downloads` `.downloads2` `.encode` `.mux` `.minfo` `.dump` `.temp` | Working directories
`LOG_FILE_NAME` | `logs.txt` | Log file
`JSON_LOGFILE` | `false` | Write the log file as JSON
`AUTO_RESTART_INTERVAL` | `24` | Restart the bot every this many hours; `0` disables
`FORCE_RESTART_ON_RPC_FAILURE` | `false` | Restart the Telegram client when an upload loses its connection, instead of waiting for it to reconnect
`ENABLE_PPROF` | `false` | Serve Go pprof profiles on `localhost:6060`
`UPSTREAM_REPO` | `Nubuki-all/Enc-Go` | GitHub repo (`owner/repo` or URL) whose `latest` release `/update` installs
`UPSTREAM_TOKEN` | | GitHub token, needed by `/update` while `UPSTREAM_REPO` is private

`COMPATIBILITY_MODE`, `EXTENDED_CAPTIONS`, `MI_IN_CAPTION`, `OVR`, `DYNO`, `ALWAYS_DEPLOY_LATEST`, `UPSTREAM_BRANCH`, `FLOOD_SLEEP_THRESHOLD` and `WORKERS` are read or listed but currently have no effect.

## Commands

Send any command with `-h` or `--help` for its options. With `CMD_SUFFIX` set, add the suffix to every command.

**General**
Command | Description
:--- | :---
`/start`, `/help` | Check the bot is up and get usage
`/ping` | Ping
`/status` | Bot and system status
`/anime` | Anime info from AniList
`/airing` | Airing info from AniList

**Queue**
Command | Description
:--- | :---
`/add` | Queue a replied video
`/l` | Queue a direct link (aria2) or Mega link
`/ql` | Queue a torrent or magnet link (qBittorrent)
`/yl` | Queue a link via yt-dlp
`/queue` | List the queue
`/editq` | Edit a queued item (name, filters, version, AniList)
`/select` | Select files from a queued torrent to encode
`/batch` | Preview and edit batches
`/clear` | Remove queued items by id, range or `all`
`/cancelall` | Cancel encodes, mux, transfers and the queue (owner only)
`/pause` | Pause encoding, indefinitely or for a time
`/v` | Turn version tags (v2, v3…) on or off

**Encode settings**
Command | Description
:--- | :---
`/get` `/set` `/reset` | View, set or reset the encode commands (`-2`…`-4` for the extra ones)
`/mget` `/mset` | View, set, reset or clear `MUX_ARGS`
`/parse` | Turn caption or AniList parsing on or off

**Names, filters and tags**
Command | Description
:--- | :---
`/setrename` | Set the custom rename format
`/name` `/vname` `/delname` | Add, view and delete rename filters by anime title
`/filter` `/vfilter` `/delfilter` | Add, view and delete the name/caption filter
`/mtag` `/etag` | Auto-tag files matching a name or release group; view or edit them
`/xtag` `/extag` | Auto-tag captions by release group and file name; view or delete them

**Transfers**
Command | Description
:--- | :---
`/download`, `/dl` | Download a replied file or a link to the server
`/upload`, `/ul` | Upload a local file, folder or link to Telegram
`/rename` | Download, rename and re-upload a file or link
`/mux` | Remux a file with new metadata, subtitles or default tracks
`/m` | MediaInfo of a replied file or link
`/list` | List the files in a link or torrent
`/listv` | List the videos at a yt-dlp link

**Channels and RSS**
Command | Description
:--- | :---
`/forward` | Forward a replied message to `FCHANNEL`
`/editmedia` | Replace the media of a channel message with a replied file
`/rss` | Subscribe (`-s`), edit (`-e`), delete (`-d`), list (`-l`), fetch (`-g`), list chats (`-c`) or roll back (`-r`) feeds

RSS feeds take include (`--inf`) and exclude (`--exf`) keywords, target chats and topics, and a command to run on each new item. By default an item counts as already sent if its title or link matches the last one; add `--match-both` to require both, so re-releases with the same title are picked up.

**Owner and developer**
Command | Description
:--- | :---
`/permit` `/unpermit` | Add or remove a temporary user
`/restart` | Restart the bot
`/update` | Replace the bot with the latest release's binary and restart; does nothing if already on it
`/nuke` | Save state, clear qBittorrent and shut the bot down
`/bash` `/sh` | Run a shell command
`/eval` | Run Go code inside the bot
`/logs` | Get the log file
`/top` | Top processes by CPU

## Source

- **[A fork of the original Enc, rewritten in Go](https://github.com/Nubuki-all/Enc)**
- **[A heavily modified fork of Danish CompressorQueue](https://github.com/1Danish-00/CompressorQueue)**
