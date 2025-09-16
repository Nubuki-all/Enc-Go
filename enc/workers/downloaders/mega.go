package downloaders

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/types"
	"enc/internal/clients/mega"
	"enc/internal/logger"
	"enc/utils"
	"enc/utils/random"
	"enc/utils/tele"
)

type MegaDownload struct {
	*types.TransferInfo
	Size         int64
	msg          *tele.Message
	client       *mega.Mega
	downloadList []*mega.Download
	err          error
	completed    bool
	// timeout      bool
	ufstr    string
	fstr     string
	name     string
	dir      string
	selected string
	uri      string
}

func NewMegaDl(uri, dir, selected string, msg *tele.Message, tInfo *types.TransferInfo) *MegaDownload {
	conf := config.Conf
	return &MegaDownload{
		TransferInfo: tInfo,
		msg:          msg,
		client:       bot.MegaClient,
		dir:          dir,
		selected:     selected,
		ufstr:        conf.UnfinishedProgressStr,
		fstr:         conf.FinishedProgressStr,
		uri:          uri,
	}
}

func (dl *MegaDownload) Start() error {
	if dl.client == nil {
		return fmt.Errorf("mega Client is inactive")
	}
	rconf := config.Rconf
	dlTimeout := rconf.MDLTimeout * time.Second
	// progressInterval := rconf.ProgresUpdateInterval * time.Second
	// updateInterval := 1 * time.Second

	var ctx context.Context
	var cancel context.CancelFunc

	if dlTimeout == 0 {
		ctx, cancel = context.Background(), func() {}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), dlTimeout)
	}

	defer cancel()
	files, err := dl.client.ListFilesFromLink(dl.uri)
	if err != nil {
		return err
	}
	if dl.selected != "" {
		err := dl.Select(files)
		if err != nil {
			return err
		}
	}
	if len(files) > 1 {
		dl.dir = filepath.Join(dl.dir, files[0].Parent)
		dl.Path = dl.dir
	} else {
		dl.Path = files[0].Name
	}
	dl.downloadList, err = dl.client.NewDownloadFromLink(dl.uri)
	if err != nil {
		return err
	}
	for _, download := range dl.downloadList {
		if err := dl.downloadFile(ctx, download); err != nil {
			return err
		}
	}
	return nil
}

type ProgressState struct {
	Total   int64
	Current int64
	Name    string
}

func (dl *MegaDownload) downloadFile(ctx context.Context, d *mega.Download) error {
	path := filepath.Join(dl.dir, d.Name)
	fileName := filepath.Base(d.Name)
	_, err := os.Stat(path)
	if os.IsExist(err) {
		err = os.Remove(path)
		if err != nil {
			return err
		}
	}

	outfile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	var current int64
	workers := 4
	workch := make(chan int)
	doneCh := make(chan struct{})
	cancelCh := make(chan struct{})
	errch := make(chan error, workers)
	progress := make(chan int64)
	progressInterval := config.Rconf.ProgresUpdateInterval * time.Second
	size := d.Size
	start := time.Now()
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		ticker := time.NewTicker(progressInterval)
		defer ticker.Stop()
		defer wg.Done()

		var stored bool
	Loop:
		for {
			select {
			case <-ctx.Done():
				break Loop
			case chunk := <-progress:
				current += chunk
				dl.ProgressCallback(start, fileName, int64(size), current)
				break Loop
			case <-doneCh:
				break Loop
			}
		}

		for {
			select {
			case <-ctx.Done():
				// timeout
				return
			case chunk := <-progress:
				if dl.IsCancelled {
					close(cancelCh)
					return
				}
				current += chunk
				stored = true
			case <-ticker.C:
				if dl.IsCancelled {
					close(cancelCh)
					return
				}
				if stored {
					dl.ProgressCallback(start, fileName, int64(size), current)
				}
			case <-doneCh:
				return
			}
		}
	}()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Wait for work blocked on channel
			for id := range workch {
				select {
				case <-ctx.Done():
					errch <- ctx.Err()
					return
				case <-cancelCh:
					return
				default:
				}
				if err := ctx.Err(); err != nil {
					errch <- err
					return
				}
				chunk, err := d.DownloadChunk(id)
				if err != nil {
					errch <- err
					return
				}

				chkStart, _, err := d.ChunkLocation(id)
				if err != nil {
					errch <- err
					return
				}

				_, err = outfile.WriteAt(chunk, chkStart)
				if err != nil {
					errch <- err
					return
				}

				progress <- int64(len(chunk))
			}
		}()
	}

	// Place chunk download jobs to chan
	err = nil
	for id := 0; id < d.Chunks() && err == nil; {
		select {
		case workch <- id:
			id++
		case err = <-errch:
		}
	}
	close(workch)
	close(doneCh)

	wg.Wait()

	closeErr := outfile.Close()
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	return d.Finish()
}

func (dl *MegaDownload) ProgressCallback(start time.Time, name string, total, current int64) {
	remainingSize := total - current
	diff := time.Since(start).Round(time.Second).Seconds()
	speed := current / int64(diff)
	var percentage float64
	var timeToCompletion string
	elapsedTime := utils.TimeFormatter(diff)
	if speed > 0 {
		timeToCompletion = utils.TimeFormatter(float64(remainingSize) / float64(speed))
	}
	if total > 0 {
		percentage = float64(current * 100 / total)
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
	msgHead := fmt.Sprintf("**Downloading:**\n`%s`\n**via:** Mega", name)
	txt := msgHead + "\n" + tmp
	_, _ = dl.msg.EditText(txt)
}

func (dl *MegaDownload) Select(files []mega.File) error {
	if len(files) == 1 {
		return nil
	}
	var fileID string
	sl, _ := strconv.Atoi(dl.selected)
	if sl > (len(files) - 1) {
		return fmt.Errorf("mega: file_id not found")
	}
	for x, file := range files {
		if x == sl {
			fileID = file.Hash
			break
		}
	}
	dl.uri = strings.TrimSuffix(dl.uri, "/")
	s := strings.Split(dl.uri, "mega.nz")
	s = strings.Split(s[1], "/")
	if len(s) == 5 {
		index := strings.LastIndex(dl.uri, "/folder/")
		if index != -1 {
			dl.uri = dl.uri[:index] + "/folder/" + fileID
		}
	} else {
		dl.uri = dl.uri + "/folder/" + fileID
	}
	return nil
}

func (dl *MegaDownload) Clean() {
	if len(dl.downloadList) == 0 {
		logger.WarnW("Aria2 Dl: Calling Clean on a download that exited early")
		return
	}
	_ = os.RemoveAll(dl.Path)
}
