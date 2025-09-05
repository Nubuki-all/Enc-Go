package downloaders

import (
	"context"
	"fmt"
	//"io"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/siku2/arigo"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type Aria2Download struct {
	*types.TransferInfo
	Size      int64
	msg       *tele.Message
	client    *arigo.Client
	err       error
	completed bool
	// timeout      bool
	ufstr        string
	fstr         string
	name         string
	dir          string
	gid          string
	selected     string
	selectedName string
	selectedPath string
	uri          string
}

func NewAria2Dl(uri, dir, selected string, msg *tele.Message, tInfo *types.TransferInfo) *Aria2Download {
	conf := config.Conf
	return &Aria2Download{
		TransferInfo: tInfo,
		msg:          msg,
		client:       bot.Aria2Client,
		dir:          dir,
		selected:     selected,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
		uri:          uri,
	}
}

func (dl *Aria2Download) Start() error {
	if dl.client == nil {
		return fmt.Errorf("aria2 Client is inactive")
	}
	rconf := config.Rconf
	dlTimeout := rconf.ADLTimeout * time.Second
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

	resp, err := dl.client.AddURI(
		arigo.URIs(dl.uri), &arigo.Options{
			Dir: dl.dir,
		})
	if err != nil {
		return err
	}
	gid := resp.GID
	dl.gid = gid
	status, err := resp.TellStatus()
	if err != nil {
		return err
	}
	if dl.selected != "" {
		metaTimeout := 20 * time.Second
		metaCtx, metaCancel := context.WithTimeout(ctx, metaTimeout)
		defer metaCancel()

		// poll until FollowedBy is present or metaCtx expires
		for len(status.FollowedBy) == 0 {
			// check context
			select {
			case <-metaCtx.Done():
				return fmt.Errorf("could not fetch selected torrent (timeout waiting for metadata)")
			default:
			}
			time.Sleep(1 * time.Second) // small backoff
			status, err = dl.client.TellStatus(gid)
			if err != nil {
				return fmt.Errorf("TellStatus while waiting for metadata: %w", err)
			}
		}
		gid := status.FollowedBy[0]
		status, err = dl.client.TellStatus(gid)
		if err != nil {
			return fmt.Errorf("TellStatus for child gid: %w", err)
		}
		err = dl.Select(status.GID)
		if err != nil {
			return fmt.Errorf("Select file: %w", err)
		}
	}
	go func() {
		uTicker := time.NewTicker(updateInterval)
		pTicker := time.NewTicker(progressInterval)
		defer uTicker.Stop()
		defer pTicker.Stop()

		curGid := dl.ProgressCallback(start, gid)
		for {
			select {
			case <-ctx.Done():
				// timeout or external cancel
				done <- fmt.Errorf("download timed out/ctx done: %w", ctx.Err())
				return
			case <-pTicker.C:
				if curGid == "" {
					done <- nil
					return
				}
				curGid = dl.ProgressCallback(start, curGid)
			case <-uTicker.C:
				if curGid == "" {
					done <- nil
					return
				}
				curGid = dl.UpdateCallback(curGid)
			}
			// continue loop
		}
	}()

	var finalErr error
	select {
	case finalErr = <-done:
	case <-ctx.Done():
		return fmt.Errorf("download context done: %w", ctx.Err())
	}

	if finalErr != nil {
		return finalErr
	}
	if dl.selected != "" && !dl.IsCancelled { // if IsCancelled is atomic.Bool: !dl.IsCancelled.Load()
		finalStatus, sErr := dl.client.TellStatus(gid)
		if sErr != nil {
			return sErr
		}
		arigo.RemoveUnselectedFiles(finalStatus)
	}

	return nil
}

func (dl *Aria2Download) ProgressCallback(start time.Time, gid string) string {
	if dl.PauseProgress {
		return dl.UpdateCallback(gid)
	}
	status, err := dl.client.TellStatus(gid)
	if err != nil {
		dl.err = err
		return ""
	}
	if len(status.FollowedBy) > 0 {
		status, err = dl.client.TellStatus(status.FollowedBy[0])
		if err != nil {
			dl.err = err
			return ""
		}
	}
	if status.Status == arigo.StatusError {
		dl.err = fmt.Errorf("Aria2 Download failed: %s", status.ErrorMessage)
		return ""
	}
	if status.Status == arigo.StatusCompleted {
		dl.completed = true
		return ""
	}
	if dl.IsCancelled {
		err = dl.client.Remove(gid)
		if err != nil {
			dl.err = err
		}
		return ""
	}
	var name string
	dl.name, _ = arigo.GetDownloadName(status)
	if dl.selectedName != "" {
		name = dl.selectedName
	} else {
		name = dl.name
	}
	total := int64(status.TotalLength)
	current := int64(status.CompletedLength)
	remainingSize := total - current
	speed := int64(status.DownloadSpeed)
	var percentage float64
	var timeToCompletion string

	diff := time.Since(start).Round(time.Second).Seconds()
	if total > 0 {
		percentage = float64(current * 100 / total)
	}
	elapsedTime := utils.TimeFormatter(diff)
	if remainingSize > 0 && speed > 0 {
		timeToCompletion = utils.TimeFormatter(float64(remainingSize) / float64(speed))
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
	msgHead := "`Download Pending…`"
	if !strings.HasSuffix(name, ".torrent") {
		if dl.selectedPath != "" {
			dl.Path = dl.selectedPath
		} else {
			dl.Path = filepath.Join(status.Dir, name)
		}
		msgHead = fmt.Sprintf("**Downloading:**\n`%s`\n**via:** ", name)
		if status.BitTorrent.Info.Name != "" {
			msgHead += "Torrent."
		} else {
			msgHead += "Direct Link."
		}
	}
	txt := msgHead + "\n" + tmp
	_, _ = dl.msg.EditText(txt)
	return status.GID
}

func (dl *Aria2Download) Select(gid string) (err error) {
	err = dl.client.Pause(gid)
	if err != nil {
		return err
	}
	err = dl.client.ChangeOptions(
		gid,
		arigo.Options{
			SelectFile: dl.selected,
		},
	)
	if err != nil {
		return err
	}
	err = dl.client.Pause(gid)
	if err != nil {
		return err
	}
	status, err := dl.client.TellStatus(gid)
	if err != nil {
		return err
	}
	for _, file := range status.Files {
		if file.Selected {
			dl.selectedPath = file.Path
			dl.selectedName = filepath.Base(file.Path)
		}
	}
	return nil
}

func (dl *Aria2Download) UpdateCallback(gid string) string {
	status, err := dl.client.TellStatus(gid)
	if err != nil {
		dl.err = err
		return ""
	}
	if len(status.FollowedBy) > 0 {
		status, err = dl.client.TellStatus(status.FollowedBy[0])
		if err != nil {
			dl.err = err
			return ""
		}
	}
	if status.Status == arigo.StatusError {
		dl.err = fmt.Errorf("Aria2 Download failed: %s", status.ErrorMessage)
		return ""
	}
	if status.Status == arigo.StatusCompleted {
		dl.completed = true
		return ""
	}
	if dl.IsCancelled {
		err = dl.client.Remove(gid)
		if err != nil {
			dl.err = err
		}
		return ""
	}
	var name string
	if dl.selectedName != "" {
		name = dl.selectedName
	} else {
		name, _ = arigo.GetDownloadName(status)
	}
	if !strings.HasSuffix(name, ".torrent") {
		if dl.selectedPath != "" {
			dl.Path = dl.selectedPath
		} else {
			dl.Path = filepath.Join(status.Dir, name)
		}
	}
	return status.GID
}

func (dl *Aria2Download) Clean() {
	if dl.gid == "" {
		logger.WarnW("Aria2 Dl: Calling Clean on a download that exited early")
		return
	}
	status, err := dl.client.TellStatus(dl.gid)
	if err != nil {
		logger.Error(err)
		return
	}
	var statuses []arigo.Status
	for _, gid := range status.FollowedBy {
		status, err := dl.client.TellStatus(gid)
		if err == nil {
			statuses = append(statuses, status)
		}
	}
	statuses = append(statuses, status)
	dl.client.DeleteDownloads(statuses, false, true, true)
}
