# Migrating from Enc to Enc-Go

[Enc](https://github.com/Nubuki-all/Enc), the Python bot, is no longer maintained. Enc-Go is its rewrite in Go and its replacement. Most settings and commands carry over, but a few things need attention. This guide lists all of them.

## Before you start

- Stop the Enc bot first. Two bots using the same bot token fight over Telegram updates.
- Note anything you set at runtime that you want to keep: queued items, RSS feeds, filters, custom rename format, `/set` encode commands and `MUX_ARGS`, and temporary users. **None of this carries over** (see [Saved data](#saved-data)).

## 1. Install Enc-Go

Enc-Go is a single executable. It downloads its own FFmpeg, aria2, qBittorrent-nox, MediaInfo, anitomy and yt-dlp into `.bin/` the first time it starts, so you don't need Enc's Python environment, `requirements.txt` or `run.sh`.

- **Release binary:** download the executable for your platform from [releases](https://github.com/Nubuki-all/Enc-Go/releases) and run it from the directory holding your `.env`.
- **Docker:** `docker build . -t enc && docker run enc`, with your `.env` next to the `Dockerfile`.
- **From source:** Go 1.26 or newer, `go build -o enc .`, then `./enc`.

Give Enc-Go its own directory rather than running it inside Enc's checkout. The files don't clash, but a clean directory keeps the old pickles and Pyrogram sessions out of the way.

## 2. Bring over your `.env`

Start from your Enc `.env`; the format is the same (`KEY=value`, no quotes). Then make these changes.

### Must change

**`FFMPEG`, `FFMPEG2`, `FFMPEG3`, `FFMPEG4`: placeholders.** Enc used two `"{}"` placeholders. Enc-Go uses `{input}` and `{output}` and quotes the paths itself, so drop the quotes:

```diff
-FFMPEG=ffmpeg -i "{}" -preset ultrafast -c:v libx265 -crf 27 -map 0:v -c:a aac -map 0:a -c:s copy -map 0:s? "{}"
+FFMPEG=ffmpeg -i {input} -preset ultrafast -c:v libx265 -crf 27 -map 0:v -c:a aac -map 0:a -c:s copy -map 0:s? {output}
```

An old command left unchanged makes every encode fail.

**`DBNAME`: use a new database if you use MongoDB.** See [Saved data](#saved-data). Either delete the `DBNAME` line (Enc-Go defaults to `ENC-GO`, Enc defaulted to `ENC`) or set a new name.

**`UPLOAD_AS_VIDEO`: replaced by `UPLOAD_AS_DOCUMENT`, with the meaning flipped.**

Enc | Enc-Go
:--- | :---
`UPLOAD_AS_VIDEO=False` (default) | `UPLOAD_AS_DOCUMENT=True` (default)
`UPLOAD_AS_VIDEO=True` | `UPLOAD_AS_DOCUMENT=False`

`UPLOAD_VIDEO_AS_SPOILER` still needs videos, so it now needs `UPLOAD_AS_DOCUMENT=False`.

### Remove

These have no effect in Enc-Go:

- `TG_DL_CLIENT`, `TG_UL_CLIENT`: Enc-Go has one built-in Telegram client.
- `WORKERS`, `COMPATIBILITY_MODE`, `EXTENDED_CAPTIONS`, `MI_IN_CAPTION`, `OVR`, `DYNO`, `FLOOD_SLEEP_THRESHOLD`: still read but unused.

### Unchanged

Everything else keeps its name and meaning:
- **Basics:** `APP_ID`, `API_HASH`, `BOT_TOKEN`, `OWNER`, `TEMP_USERS`, `DEV`, `CMD_SUFFIX`, `DATABASE_URL`.
- **Captions and naming:** `ENCODER`, `RELEASER`, `CAP_DECO`, `C_LINK`, `THUMBNAIL`, `CUSTOM_RENAME`, `FILENAME_AS_CAPTION`, `USE_ANILIST`, `USE_CAPTION`.
- **Channels:** `FCHANNEL`, `FCHANNEL_STAT`, `FSTICKER`, `FCODEC`, `FBANNER`, `NO_BANNER`, `LOG_CHANNEL`, `LOGS_IN_CHANNEL`, `DUMP_CHANNEL`, `DUMP_LEECH`.
- **Encoding and behaviour:** `MUX_ARGS`, `LOCK_ON_STARTUP`, `ALLOW_ACTION`, `NO_TEMP_PM`, `PODI`, `REPORT_FAILED`, `REPORT_FAILED_DL`, `REPORT_FAILED_ENC`, `FINISHED_PROGRESS_STR`, `TELEGRAPH_AUTHOR`, `DL_STUFF`.
- **RSS:** `RSS_CHAT`, `RSS_DELAY`, `RSS_DIRECT`.
- **Download services:** `ARIA2_PORT`, `QBIT_PORT`, `QBIT_PORT2`, `QBIT_TIMEOUT`, `ARIA2_DL_TIMEOUT`, `QBIT_DL_TIMEOUT`.

### New in Enc-Go (optional)

A few worth knowing about; the [README](../README.md#configuration) lists them all:

- **Encoding:** `SEQUENTIAL_JOBS` and `MAX_SEQUENTIAL_JOBS` run the extra encode commands in parallel.
- **Queueing:** `AUTO_QUEUE_VID` queues videos sent to the bot in private chat.
- **RSS:** `RSS_CHAT_TOPIC` sends RSS items to a topic.
- **Google Drive:** `GDRIVE_SA_JSON` is a service account for private Drive links. Public Drive links work without it.
- **Updates:** `UPSTREAM_REPO` and `UPSTREAM_TOKEN` are used by `/update`.
- **Timeouts:** `MEGA_DL_TIMEOUT`, `YT_DL_TIMEOUT`, `YTDLP_TIMEOUT`, `TG_UL_TIMEOUT`.
- **Running:**
  - `NO_EXTERNAL_DL_SERVICE`, `AUTO_RESTART_INTERVAL` (restarts every 24 h by default; `0` turns it off), `FORCE_RESTART_ON_RPC_FAILURE`, `ENABLE_PPROF`;
  - working directories: `DOWNLOAD_DIR`, `ENCODE_DIR`, `TEMP_DIR` and others;
  - storage and logging: `LOCAL_DB_PATH`, `JSON_LOGFILE`.

## Saved data

Enc stored its state as Python pickles, in MongoDB and in local `.pkl` files alike. Enc-Go can't read pickles, so state is **not migrated**:

- the queue and batches;
- RSS feeds;
- name, release and rename filters, and the custom rename format;
- encode commands changed with `/set` and `MUX_ARGS` changed with `/mset`;
- temporary users added with `/permit`.

Set these up again in Enc-Go. Values in `.env` (like `FFMPEG`, `MUX_ARGS`, `TEMP_USERS`, `CUSTOM_RENAME`) are read fresh, so anything you keep there carries over automatically.

**MongoDB users: don't point Enc-Go at Enc's database.** Both use the same collection names (`queue`, `code`, `filter`, `rss`, `users`) and the same document id (your bot's id). In a shared database, Enc-Go tries to read Enc's pickled documents, fails, and then overwrites them in its own format. Use a new `DBNAME` as described above. Once you no longer need Enc's database, you can drop it (by default it's called `ENC`).

**Without MongoDB**, Enc used `.local_queue.pkl`, `.local_bqueue.pkl`, `.local_rssdb.pkl`, `.t_users.pkl` and `.crf.pkl`. Enc-Go uses an SQLite file, `.encbot.db` by default. You can delete the `.pkl` files.

The Telegram login isn't carried over either: Enc-Go keeps its own session in `.encbot.session` and logs in with `BOT_TOKEN` on first start.

## Commands

Most commands work as before. The short forms carry over; some long aliases and a few commands are gone.

Enc | Enc-Go
:--- | :---
`/leech`, `/l` | `/l`
`/qbleech`, `/ql` | `/ql`
`/mediainfo`, `/m` | `/m`
`/rename`, `/rn` | `/rename`
`/batch`, `/gb` | `/batch`
`/select`, `/s` | `/select`
`/add`, `/releech` | `/add`
`/clean`, `/cancelall` | `/cancelall` (now owner-only)
`/lock`, `/pause` | `/pause`
`/fforward`, `/forward` | `/forward`
`/download`, `/dl` | `/download`, `/dl`
`/upload`, `/ul` | `/upload`, `/ul`
`/showthumb` | removed
`/groupenc` | removed
`/peval` | removed: `/eval` runs Go code now, not Python

**New in Enc-Go:**
- **Queueing:** `/yl` queues a link through yt-dlp, `/listv` lists a yt-dlp playlist, and `/editq` edits a queued item.
- **Tags:** `/mtag` and `/etag` auto-tag by name or release group; `/xtag` and `/extag` auto-tag captions.
- **Channels:** `/editmedia` replaces a channel message's media.
- **Dev tools:** `/sh` runs a shell command and `/top` shows the top processes.
- **Updates:** `/update` installs the latest release and restarts. Enc's update pulled from git; Enc-Go updates from GitHub releases.

`/rss` takes flags (`-s` subscribe, `-e` edit, `-d` delete, `-l` list, `-g` get, `-c` list chats, `-r` roll back); send `/rss -h` for details. Google Drive links work with `/l`, `/list`, `/download`, `/rename` and `/mux`.

Send any command with `-h` or `--help` for its options.

## Checklist

- [ ] Enc stopped
- [ ] `FFMPEG*` placeholders changed to `{input}` / `{output}`, quotes removed
- [ ] `UPLOAD_AS_VIDEO` replaced with `UPLOAD_AS_DOCUMENT` (inverted)
- [ ] A new `DBNAME` (or no `DBNAME` line) if you use MongoDB
- [ ] `TG_DL_CLIENT` / `TG_UL_CLIENT` removed
- [ ] Enc-Go started, then RSS feeds, filters and `/set` changes set up again
- [ ] A test item queued and encoded
