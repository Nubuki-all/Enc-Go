# [ENC-Go]

[![Checks](https://github.com/Nubuki-all/Enc-Go/actions/workflows/GoLint.yml/badge.svg?branch=main)](https://github.com/Nubuki-all/Enc-Go/actions/workflows/GoLint.yml)

__Still in beta…__

### Variables

___(For local/vps deployment rename [.env.sample](.env.sample) to .env and edit with your variable)___
(list of available variables isn't complete.)
---
Compulsory Variables | Explanation
:--------- | :---------------------------------------------
`APP_ID` `API_HASH` `BOT_TOKEN` | get the first two from *[Telegram.org](https://telegram.org)* and the third from *[@Botfather](https://t.me/botfather)*
`OWNER`    | input id Of allowed users with a space between each

Optional Variables | Explanation
:--------- | :---------------------------------------------
`THUMBNAIL` `ICON` | input telegraph link of a picture for use as Thumbnail, Watermark.
`FFMPEG` | input Your FFMPEG Code or Handbrake-cli code (after installing it)  with """{}""" as input and output. (Eg. __ffmpeg -i """{}""" -preset veryfast -vcodec libx265 -crf 27 """{}"""__) escape the " characters if you're deploying locally 
`TEMP_USERS` | Additional restricted users allowed to use the bot
`LOG_CHANNEL` | Input Log Group/Channel ID (bot must be an admin in target group or channel)
`DATABASE_URL` | input valid Mongodb Database Url
`ENCODER` | Encoder's name/nickname to be included in captions can also contain encode info.
`FCHANNEL` `FCHANNEL_STATS` | Input Channel id where **only** the output video will get forwarded. For the 2nd variable input message id of a message in channel to be used as live status for encodes.
`LOGS_IN_CHANNEL` type=bool | Get error logs in log channel. Turned off by default, set to True to turn on.
`CMD_SUFFIX` | Bot will only listen to commands ending with the given value
`TELEGRAPH_API` | Api to use instead of api.telegra.ph when posting mediainfo
`LOCK_ON_STARTUP` | Pause bot on startup untill pause off is used.
`ALLOW_ACTION` type=bool | Set to True or False depending on whether you want encoding chat actions enabled for bot 
---


### Commands (outdated)
---
```
start - check if bot is awake and get usage.
restart -  restart bot
update - update bot
nuke - ☢️ nuke bot
bash - /bash + command
eval - evaluate code
pause - prevent bot from encoding
peval - same as eval but with pyrogram
ping - ping!
add - add video to queue
leech - add link to queue
qbleech - add torrent link to queue
select - select files to encode in a batch
permit - add a temporary user
unpermit - removes a temporary user
queue - list queue
batch - preview batches
list - list all files in a torrent
forward - manually forward a message to fchannel
v - turn v2,3,4… on (with message) or off
download - download a file or link to bot
upload - upload from a local directory or link
rename - rename a video file/link
mediainfo - get the media info of a replied file/link
mux - remux a file
get - get current ffmpeg code
set - set custom ffmpeg code
reset - reset default ffmpeg code
filter - filter & stuff
vfilter - view filter
groupenc - allow encoding in group toggle
delfilter - delete filter
rss - edit, delete & subscribe rss feeds
anime - get anime info
airing - get anime's airing info
setrename - set custom_rename format
name - quick filter with anime_title
vname - get list of name filter
delname - delete name filter
status - 🆕 get bot's status
showthumb - 🖼️ show current thumbnail
parse - toggle parsing with captions or anilist
groupenc - turn off/on encoding in groups
cancelall - ❌ clear cached downloads & queued files
clear - clear queued files
logs - get bot logs
help - same as start
```

### Deployment:
**With Docker:**
- Install Docker
- Clone repository to your preferred location 
- Ensure you are in the proper directory with Dockerfile and .env file present
- Run:
  - `docker build . -t enc`
  - `docker run enc`

**Without Docker:**
- Grab executable for your host architecture (linux only) from releases and run

### Features:
__(Coming Soon)__

### Source 
- **[A Fork of the original Enc re-written in go](https://github.com/Nubuki-all/Enc)**
- **[An Heavily Modified Fork of Danish CompressorQueue](https://github.com/1Danish-00/CompressorQueue)**
