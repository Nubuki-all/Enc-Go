package downloaders

import (
	"context"
	"fmt"
	//"io"
	"math"
	"path/filepath"
	"strings"
	"strconv"
	"time"

	qbit "github.com/autobrr/go-qbittorrent"
	"github.com/google/uuid"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type QbitDownload struct {
	*types.TransferInfo
	Size      int64
	msg       *tele.Message
	client    *qbit.Client
	err       error
	completed bool
	// timeout      bool
	ufstr        string
	fstr         string
	name         string
	dir          string
	tag          string
	hash         string
	selected     string
	selectedName string
	selectedPath string
	uri          string
}

func NewQbtitDl(uri, dir, selected string, msg *tele.Message, tInfo *types.TransferInfo) *QbitDownload {
	conf := config.Conf
	return &QbitDownload{
		TransferInfo: tInfo,
		msg:          msg,
		client:       *qbit.Client,
		dir:          dir,
		selected:     selected,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
		uri:          uri,
	}
}

func (dl *QbitDownload) Start() (err error) {
	qb, err := getQbitClient(false)
	if err != nil {
		return fmt.Errorf("qbit Client is inactive")
	}
	dl.client = qb
	rconf := config.Rconf
	dlTimeout := rconf.QDLTimeout * time.Second
	progressInterval := rconf.ProgresUpdateInterval * time.Second
	updateInterval := 1 * time.Second

	var ctx context.Context
	var cancel context.CancelFunc

	if dlTimeout == 0 {
		ctx, cancel = context.Background(), func() {}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), dlTimeout)
	}

	defer cancel()

	done := make(chan error, 1)
	start := time.Now()

	tag := "qb_dl" + uuid.New().String()
	options := qbit.TorrentAddOptions{
		SavePath: dl.dir,
		Tags:     tag,
	}
	err = qb.AddTorrentFromUrlCtx(ctx, dl.uri, options.Prepare())
	if err != nil {
		return err
	}
	dl.tag = tag
	torrents, err := qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
	if err != nil {
		return err
	}
	metaTimeout := rconf.QBitTimeout * time.Second
	metaCtx, metaCancel := context.WithTimeout(ctx, metaTimeout)
	defer metaCancel()
	// poll until FollowedBy is present or metaCtx expires
	for len(torrents) == 0 {
		// check context
		select {
		case <-metaCtx.Done():
			return fmt.Errorf("could not fetch selected torrent (timeout waiting for metadata)")
		default:
		}
		time.Sleep(1 * time.Second) // small backoff
		torrents, err = qb.GetTorrentsCtx(metaCtx, qbit.TorrentFilterOptions{Tag: tag})
		if err != nil {
			return fmt.Errorf("qbittorrent error while waiting for metadata: %w", err)
		}
	}
	if torrents[0].State == qbit.TorrentStateMetaDl {
		_, _ = msg.EditText("`Getting torrent's metadata. Please wait…`")
	}
	for torrents[0].State == qbit.TorrentStateMetaDl {
		select {
		case <-metaCtx.Done():
			return fmt.Errorf("could not fetch selected torrent (timeout waiting for metadata)")
		default:
		}
		time.Sleep(1 * time.Second) // small backoff
		torrents, err = qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
		if err != nil {
			return fmt.Errorf("qbittorrent error while waiting for download: %w", err)
		}
	}
	if torrents[0].State == qbit.TorrentStateError {
		return fmt.Errorf("an unknown error occured")
	}
	dl.hash = torrents[0].Hash
	dl.name = torrents[0].Name
	dl.Path = torrents[0].ContentPath
	if dl.selected != "" {
		err = dl.Select(ctx, dl.hash)
		if err != nil {
			return fmt.Errorf("Select file: %w", err)
		}
	}

	go func() {
		uTicker := time.NewTicker(updateInterval)
		pTicker := time.NewTicker(progressInterval)
		defer uTicker.Stop()
		defer pTicker.Stop()

		ok := dl.ProgressCallback(ctx, start, tag)
		for {
			select {
			case <-ctx.Done():
				// timeout or external cancel
				done <- fmt.Errorf("download timed out after %s", utils.TimeFormatter(dlTimeout.Seconds()))
				return
			case <-pTicker.C:
				if !ok {
					done <- nil
					return
				}
				ok = dl.ProgressCallback(ctx, start, tag)
			case <-uTicker.C:
				if !ok {
					done <- nil
					return
				}
				ok = dl.UpdateCallback(ctx, tag)
			}
			// continue loop
		}
	}()

	var finalErr error
	select {
	case finalErr = <-done:
	case <-ctx.Done():
		return fmt.Errorf("download timed out after %s", utils.TimeFormatter(dlTimeout.Seconds()))
	}

	if finalErr != nil {
		return finalErr
	}
	return nil
}

func (dl *QbitDownload) ProgressCallback(ctx context.Context, start time.Time, tag string) (ok bool) {
	if dl.PauseProgress {
		return dl.UpdateCallback(tag)
	}
	qb := dl.client
	torrents, err = qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
	if err != nil {
		dl.err = err
		return
	}
	torrent = torrents[0]
	switch torrent.State {
	case qbit.TorrentStateError:
		dl.err = fmt.Errorf("an unknown error occured")
		return
	case qbit.TorrentStatePausedUp:
		dl.completed = true
		return
	case qbit.TorrentStateStoppedUp:
		dl.completed = true
		return
	}
	if dl.IsCancelled {
		err = qb.PauseCtx(ctx, []string{torrent.Hash})
		if err != nil {
			dl.err = err
		}
		return
	}

	ok = true
	var name string
	if dl.selectedName != "" {
		name = dl.selectedName
	} else {
		name = torrent.Name
	}

	total := torrent.Size
	current := torrent.Completed
	remainingSize := total - current
	speed := torrent.DlSpeed
	eta := torrent.ETA
	var percentage float64
	var timeToCompletion string 

	diff := time.Since(start).Round(time.Second).Seconds()
	if total > 0 {
		percentage = float64(current * 100 / total)
	}
	elapsedTime := utils.TimeFormatter(diff)
	if eta > 0 {
		timeToCompletion = utils.TimeFormatter(float64(eta))
	}
	filled := int(math.Floor(percentage / 10.0))
	finStr := dl.fstr
	unfinStr := dl.ufstr
	if finStr == "" {
		finStr = random.EnHearts()
	}
	bar := strings.Repeat(finStr, filled) + strings.Repeat(unfinStr, 10-filled)
	progress := fmt.Sprintf("```\n%s```\n**Progress:** `%.2f%%`\n", bar, percentage)
	if timeToCompletion == "" {
		timeToCompletion = "0 s"
	}
	if elapsedTime == "" {
		elapsedTime = "0 s"
	}
	tmp := fmt.Sprintf(
		"%s`%s of %s`\n**Speed:** `%s/s`\n**Remains:** `%s`\n**ETA:** `%s`\n**Elapsed:** `%s`\n",
		progress,
		utils.VHBS(current),
		utils.VHBS(total),
		utils.VHBS(speed),
		utils.VHBS(remainingSize),
		timeToCompletion,
		elapsedTime,
	)
	var msgHead string
	switch torrent.State {
	case qbit.TorrentStateCheckingResumeData:
		msgHead = "`Starting Download…`"
	case qbit.TorrentStateStalledDl:
		msgHead = "`Download stalled…`"
	case qbit.TorrentStatePausedDl:
		msgHead = "`Download Paused.`"
	case qbit.TorrentStateDownloading:
		msgHead = "**Downloading:**\n`" + name + "`"
		msgHead += "\n**via:** Torrent."
	default:
		msgHead = "`..................`"
	}
	txt := msgHead + "\n" + tmp
	_, _ = dl.msg.EditText(txt)
	return
}

func (dl *QbitDownload) Select(ctx context.Context, hash string) (err error) {
	qb := dl.client
	err = qb.PauseCtx(ctx, []string{hash})
	if err != nil {
		return err
	}
	files, err = getFilesFromQb(ctx, qb, hash)
	if err != nil {
		return err
	}
	s, _ := strconv.Atoi(dl.selected)
	if s > (len(files) - 1) {
		return fmt.Errorf("qbittorrent: file_id not found.")
	}
	fileIDs := make([]string, 0, len(files)-1)
	for i, _ := range files {
		if i == s {
			continue
		}
		fileIDs = append(fileIDs, strconv.Itoa(i))
	}
	err = qb.SetFilePriorityCtx(
		ctx,
		hash,
		strings.Join(fileIDs, "|"),
		0,
	)
	if err != nil {
		return err
	}
	err = qb.ResumeCtx(ctx, []string{hash})
	if err != nil {
		return err
	}
	dl.selectedPath = filepath.Join(dl.dir, files[s])
	dl.selectedName = filepath.Base(dl.selectedPath)
	dl.Path, dl.name = dl.selectedPath, dl.selectedName
	return nil
}

func (dl *QbitDownload) UpdateCallback(ctx context.Context, tag string) (ok bool) {
	qb := dl.client
	torrents, err = qb.GetTorrentsCtx(ctx, qbit.TorrentFilterOptions{Tag: tag})
	if err != nil {
		dl.err = err
		return
	}
	torrent = torrents[0]
	switch torrent.State {
	case qbit.TorrentStateError:
		dl.err = fmt.Errorf("an unknown error occured")
		return
	case qbit.TorrentStatePausedUp:
		dl.completed = true
		return
	case qbit.TorrentStateStoppedUp:
		dl.completed = true
		return
	}
	if dl.IsCancelled {
		err = qb.PauseCtx(ctx, []string{torrent.Hash})
		if err != nil {
			dl.err = err
		}
		return
	}
	ok = true
	return
}

func (dl *QbitDownload) Clean() {
	if dl.tag == "" {
		logger.WarnW("Qbittorrent Dl: Calling Clean on a download that exited early")
		return
	}
	ctx := dl.msg.Ctx
	err = qb.DeleteTagsCtx(ctx, []string{dl.tag})
	if err != nil {
		logger.Error(err)
	}
	err = qb.DeleteTorrentsCtx(ctx, []string{dl.hash}, true)
	if err != nil {
		logger.Error(err)
	}
}
