// Package mega initiates and contains the mega client
package mega

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"

	//"crypto/rand"
	//"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	//"io/ioutil"
	"net/http"
	//"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"enc/internal/logger"

	"golang.org/x/sync/errgroup"
)

// from https://github.com/t3rm1n4l/go-mega/blob/master/mega.go#L55

const (
	APIURL             = "https://g.api.mega.co.nz"
	BaseDownloadURL    = "https://mega.co.nz"
	Retries            = 10
	DownloadWorkers    = 3
	MaxDownloadWorkers = 30
	UploadWorkers      = 1
	MaxUploadWorkers   = 30
	Timeout            = time.Second * 10
	HTTPSOnly          = false
	minSleepTime       = 10 * time.Millisecond // for retries
	maxSleepTime       = 5 * time.Second       // for retries
)

type config struct {
	baseURL   string
	retries   int
	dlWorkers int
	ulWorkers int
	timeout   time.Duration
	https     bool
}

func newConfig() config {
	return config{
		baseURL:   APIURL,
		retries:   Retries,
		dlWorkers: DownloadWorkers,
		ulWorkers: UploadWorkers,
		timeout:   Timeout,
		https:     HTTPSOnly,
	}
}

type Mega struct {
	config
	// Version of the account
	/*accountVersion int
	// Salt for the account if accountVersion > 1
	accountSalt []byte
	// Sequence number
	sn int64
	// Server state sn
	ssn string
	// Session ID
	sid string
	// Master key
	k []byte
	// User handle
	uh []byte*/
	// Filesystem object
	// FS *MegaFS
	// HTTP Client
	client *http.Client
	// Loggers
	// logf   func(format string, v ...interface{})
	// debugf func(format string, v ...interface{})
	// serialize the API requests
	apiMu sync.Mutex
	// mutex to protext waitEvents
	// waitEventsMu sync.Mutex
	// Outstanding channels to close to indicate events all received
	// waitEvents []chan struct{}
}

func New() *Mega {
	/* max := big.NewInt(0x100000000)
	bigx, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err) // this should be returned, but this is a public interface
	}*/
	cfg := newConfig()
	// mgfs := newMegaFS()
	m := &Mega{
		config: cfg,
		/*sn:     bigx.Int64(),
		FS:     mgfs,*/
		client: newHTTPClient(cfg.timeout),
	}
	return m
}

// backOffSleep sleeps for the time pointed to then adjusts it by
// doubling it up to a maximum of maxSleepTime.
//
// This produces a truncated exponential backoff sleep
func backOffSleep(pt *time.Duration) {
	time.Sleep(*pt)
	*pt *= 2
	if *pt > maxSleepTime {
		*pt = maxSleepTime
	}
}

// API request method
func (m *Mega) apiRequest(r []byte, folderID string) (buf []byte, err error) {
	var resp *http.Response
	// serialize the API requests
	m.apiMu.Lock()
	defer func() {
		// m.sn++
		m.apiMu.Unlock()
	}()

	url := m.baseURL + "/cs"
	if folderID != "" {
		url += "?n=" + folderID
	}

	sleepTime := minSleepTime // inital backoff time
	for i := 0; i < m.retries+1; i++ {
		if i != 0 {
			logger.Info("Retry API request %d/%d: %v", i, m.retries, err)
			backOffSleep(&sleepTime)
		}
		resp, err = m.client.Post(url, "application/json", bytes.NewBuffer(r))
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			// err must be not-nil on a continue
			err = errors.New("Http Status: " + resp.Status)
			_ = resp.Body.Close()
			continue
		}
		buf, err = io.ReadAll(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			continue
		}
		err = resp.Body.Close()
		if err != nil {
			continue
		}

		// at this point the body is read and closed

		if !bytes.HasPrefix(buf, []byte("[")) && !bytes.HasPrefix(buf, []byte("-")) {
			return nil, ErrBadResp
		}

		if len(buf) < 6 {
			var emsg [1]errorMsg
			err = json.Unmarshal(buf, &emsg)
			if err != nil {
				err = json.Unmarshal(buf, &emsg[0])
			}
			if err != nil {
				return buf, ErrBadResp
			}
			err = parseError(emsg[0])
			if err == ErrAgain {
				continue
			}
			return buf, err
		}

		if err == nil {
			return buf, nil
		}
	}

	return nil, err
}

// Download represents a Mega download operation.
type Download struct {
	m *Mega
	// src         *Node // Commented out in original
	ResourceURL string
	Name        string
	Size        uint64
	aesBlock    cipher.Block
	iv          []byte
	ivA32       []uint32
	macEnc      cipher.BlockMode
	metaMacTwo  []uint32
	mutex       sync.Mutex // to protect the following
	chunks      []chunkSize
	chunkMacs   [][]byte
}

var zeroIV = make([]byte, 16)

func (m *Mega) NewDownloadsFromFolderLink(url string) ([]*Download, error) {
	if url == "" {
		return nil, ErrArgs
	}
	folderInfo, err := parseMegaFolderURL(url)
	if err != nil {
		return nil, err
	}
	if folderInfo.Type == 0 {
		return nil, fmt.Errorf("not a folder link")
	}
	folderKeyA32, err := base64ToA32(folderInfo.KeyFrag)
	if err != nil {
		return nil, fmt.Errorf("decode key fragment: %w", err)
	}

	var msg [1]DownloadFolderMsg
	var res [1]DownloadFolderResp

	msg[0].Cmd = "f"
	msg[0].C = 1
	msg[0].CA = 1
	msg[0].R = 1

	request, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	result, err := m.apiRequest(request, folderInfo.ID)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(result, &res)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]FSNode)
	children := make(map[string][]string)
	var folderID string
	for _, n := range res[0].F {
		nodes[n.Hash] = n
		parent := n.Parent
		if x := strings.Split(n.Key, ":"); len(x) > 1 && n.Hash == x[0] {
			folderID = n.Hash
		}
		children[parent] = append(children[parent], n.Hash)
	}

	// handle explicit /file/... or /folder/... path in URL
	pathHandle := folderInfo.InnerFolderHandle
	sFileHandle := folderInfo.InnerFileHandle
	startHandle := folderID

	// if user passed a file handle in the URL, prune the node set to the ancestor chain
	if sFileHandle != "" {
		prunedNodes, prunedChildren, perr := pruneNodesToPath(nodes, sFileHandle, startHandle)
		if perr != nil {
			return nil, fmt.Errorf("prune path: %w", perr)
		}
		nodes = prunedNodes
		children = prunedChildren
		// Ensure normalized children keys still use folderInfo.ID for root if needed
		if _, ok := children[startHandle]; !ok {
			// try to rebuild a top-level key if necessary
			if cs, ok2 := children[""]; ok2 {
				children[startHandle] = cs
				delete(children, "")
			}
		}
	}
	if pathHandle != "" {
		if _, ok := nodes[pathHandle]; ok {
			startHandle = pathHandle
		}
	}

	var downloads []*Download
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g, _ := errgroup.WithContext(ctx)

	var walk func(curHandle string, relPath string) error
	walk = func(curHandle string, relPath string) error {
		for _, child := range children[curHandle] {
			node, ok := nodes[child]
			if !ok {
				continue
			}
			// skip if node has no Key - can't decrypt
			if node.Key == "" {
				continue
			}

			// parse encrypted key (last colon-separated part)
			parts := strings.Split(node.Key, ":")
			encB64 := parts[len(parts)-1]
			encA32, derr := base64ToA32(encB64)
			if derr != nil {
				logger.Warn("can't decode key for this node: " + derr.Error())
				continue
			}

			// decrypt node key using current folder key
			decA32, derr := decryptKeyA32(encA32, folderKeyA32)
			if derr != nil {
				logger.Warn("skipped 1;  cannot decrypt with current folder key; " + derr.Error())
				continue
			}

			// if folder -> recurse using the decrypted child folder key (decA32)
			if node.T == 1 {
				// try to obtain a friendly folder name from attrs (if present)
				fName := node.Hash
				if node.Attr != "" {
					kBytes, err := a32ToBytes(decA32)
					if err != nil {
						logger.Warn(err)
						continue
					}
					if attrs, derr2 := decryptAttr(kBytes, node.Attr); derr2 == nil {
						if nname := attrs.Name; nname != "" {
							fName = nname
						}
					}
				}
				newRel := filepath.Join(relPath, filepath.Base(fName))
				g.Go(func() error {
					if err := walk(node.Hash, newRel); err != nil{
						return err
					} else {
						return nil
					}
				})
				continue
			}

			// node.T == 0 => file
			if len(decA32) < 8 {
				continue
			}

			// derive file key bytes (k)
			kA32 := make([]uint32, 4)
			for i := 0; i < 4; i++ {
				kA32[i] = decA32[i] ^ decA32[i+4]
			}
			keyBytes, err := a32ToBytes(kA32)
			if err != nil {
				logger.Warn(err)
				continue
			}

			// build AES block and MAC encrypter
			aesBlock, err := aes.NewCipher(keyBytes)
			if err != nil {
				return err
			}
			macEnc := cipher.NewCBCEncrypter(aesBlock, zeroIV)

			// iv for CTR is a32_to_bytes([iv0, iv1, iv0, iv1])
			ivBytes, err := a32ToBytes([]uint32{decA32[4], decA32[5], decA32[4], decA32[5]})
			if err != nil {
				return err
			}
			var msg [1]DownloadMsg
			var res [1]DownloadResp

			msg[0].Cmd = "g"
			msg[0].G = 1
			msg[0].N = node.Hash
			if m.https {
				msg[0].SSL = 2
			}
			request, err := json.Marshal(msg)
			if err != nil {
				return err
			}
			result, err := m.apiRequest(request, folderInfo.ID)
			if err != nil {
				return err
			}

			err = json.Unmarshal(result, &res)
			if err != nil {
				return err
			}
			if len(res) == 0 {
				continue
			}
			// DownloadResp has an embedded error in it for some reason
			if res[0].Err != 0 {
				logger.Warn(parseError(res[0].Err))
			}

			// decrypt attributes from API response if present to get filename
			var fileName string
			if res[0].Attr != "" {
				if attrObj, derr := decryptAttr(keyBytes, res[0].Attr); derr == nil {
					if attrObj.Name != "" {
						fileName = attrObj.Name
					}
				}
			}
			if fileName == "" {
				fileName = node.Hash + ".bin"
			}

			// build chunks and download URL
			downloadURL := res[0].G
			if m.https && strings.HasPrefix(downloadURL, "http://") {
				downloadURL = "https://" + strings.TrimPrefix(downloadURL, "http://")
			}
			chunks := getChunkSizes(int64(res[0].Size))

			// build Download and set name to include parent path (relPath)
			relName := fileName
			if relPath != "" {
				relName = filepath.Join(relPath, fileName)
			}
			d := &Download{
				m:           m,
				ResourceURL: downloadURL,
				Name:        relName,
				Size:        res[0].Size,
				aesBlock:    aesBlock,
				iv:          ivBytes,
				ivA32:       []uint32{decA32[4], decA32[5]},
				macEnc:      macEnc,
				metaMacTwo:  []uint32{decA32[6], decA32[7]},
				chunks:      chunks,
				chunkMacs:   make([][]byte, len(chunks)),
			}
			downloads = append(downloads, d)
		}
		return nil
	}
	// begin walk from the chosen startHandle using startKey (derived above)
	if err := walk(startHandle, ""); err != nil {
		cancel()
		return nil, err
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if len(downloads) == 0 {
		return nil, errors.New("no downloadable files found (or unable to decrypt keys)")
	}
	return downloads, nil
}

// GetRootFolderName returns the display name for the root or subfolder of a public folder share
func (m *Mega) GetRootFolderName(url string) (string, error) {
	if url == "" {
		return "", ErrArgs
	}
	folderInfo, err := parseMegaFolderURL(url)
	if err != nil {
		return "", err
	}
	if folderInfo.Type == 0 {
		return "", fmt.Errorf("not a folder link")
	}
	folderKeyA32, err := base64ToA32(folderInfo.KeyFrag)
	if err != nil {
		return "", fmt.Errorf("decode key fragment: %w", err)
	}

	var msg [1]DownloadFolderMsg
	var res [1]DownloadFolderResp

	msg[0].Cmd = "f"
	msg[0].C = 1
	msg[0].CA = 1
	msg[0].R = 1

	request, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	result, err := m.apiRequest(request, folderInfo.ID)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(result, &res)
	if err != nil {
		return "", err
	}
	nodes := make(map[string]FSNode)
	var folderID string
	for _, n := range res[0].F {
		nodes[n.Hash] = n
		if x := strings.Split(n.Key, ":"); len(x) > 1 && n.Hash == x[0] {
			folderID = n.Hash
		}
	}

	pathHandle := folderInfo.InnerFolderHandle
	startHandle := folderID

	if pathHandle != "" {
		if _, ok := nodes[pathHandle]; ok {
			startHandle = pathHandle
		}
	}

	rootNode := nodes[startHandle]
	parts := strings.Split(rootNode.Key, ":")
	encB64 := parts[len(parts)-1]
	encA32, derr := base64ToA32(encB64)
	if derr != nil {
		return "", fmt.Errorf("can't decode key for root node: " + derr.Error())
	}

	// decrypt node key using current folder key
	decA32, derr := decryptKeyA32(encA32, folderKeyA32)
	if derr != nil {
		return "", fmt.Errorf("cannot decrypt with folder key; " + derr.Error())
	}
	fName := rootNode.Hash
	if rootNode.Attr != "" {
		kBytes, err := a32ToBytes(decA32)
		if err != nil {
			return "", err
		}
		if attrs, derr2 := decryptAttr(kBytes, rootNode.Attr); derr2 == nil {
			if nname := attrs.Name; nname != "" {
				fName = nname
			}
		}
	}
	return fName, nil
}

// NewDownloadFromLink creates a new Download from link
//
// Call Chunks to find out how many chunks there are, then for id =
// 0..chunks-1 call DownloadChunk.  Finally call Finish() to receive
// the error status.
func (m *Mega) NewDownloadFromLink(url string) ([]*Download, error) {
	if url == "" {
		return nil, ErrArgs
	}
	if isFolder(url) {
		return m.NewDownloadsFromFolderLink(url)
	}
	id, keyFrag, err := parseMegaURL(url)
	if err != nil {
		return nil, err
	}
	fileKeyA32, err := base64ToA32(keyFrag)
	if err != nil {
		return nil, fmt.Errorf("decode key fragment: %w", err)
	}
	if len(fileKeyA32) < 8 {
		return nil, fmt.Errorf("key a32 too short")
	}
	// derive k = xor parts
	kA32 := make([]uint32, 4)
	for i := 0; i < 4; i++ {
		kA32[i] = fileKeyA32[i] ^ fileKeyA32[i+4]
	}
	// iv a32 parts
	ivA32 := fileKeyA32[4:6]
	metaMacTwo := fileKeyA32[6:8]
	// k bytes
	kBytes, err := a32ToBytes(kA32)
	if err != nil {
		logger.Warn(err)
		return nil, err
	}
	var msg [1]DownloadMsg
	var res [1]DownloadResp

	msg[0].Cmd = "g"
	msg[0].G = 1
	msg[0].P = id
	if m.https {
		msg[0].SSL = 2
	}
	key := kBytes
	request, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	result, err := m.apiRequest(request, "")
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(result, &res)
	if err != nil {
		return nil, err
	}

	// DownloadResp has an embedded error in it for some reason
	if res[0].Err != 0 {
		return nil, parseError(res[0].Err)
	}

	attr, err := decryptAttr(key, res[0].Attr)
	if err != nil {
		return nil, err
	}

	chunks := getChunkSizes(int64(res[0].Size))

	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	macEnc := cipher.NewCBCEncrypter(aesBlock, zeroIV)

	iv, err := a32ToBytes([]uint32{ivA32[0], ivA32[1], ivA32[0], ivA32[1]})
	if err != nil {
		return nil, err
	}
	downloadURL := res[0].G
	if m.https && strings.HasPrefix(downloadURL, "http://") {
		downloadURL = "https://" + strings.TrimPrefix(downloadURL, "http://")
	}

	d := &Download{
		m: m,
		// src:         src,
		ResourceURL: downloadURL,
		Name:        attr.Name,
		Size:        res[0].Size,
		aesBlock:    aesBlock,
		iv:          iv,
		ivA32:       ivA32,
		macEnc:      macEnc,
		metaMacTwo:  metaMacTwo,
		chunks:      chunks,
		chunkMacs:   make([][]byte, len(chunks)),
	}
	return []*Download{d}, nil
}

// Chunks returns The number of chunks in the download.
func (d *Download) Chunks() int {
	return len(d.chunks)
}

// ChunkLocation returns the file name of the the download.
/*func (d *Download) Name() string {
	return d.name
}

// Size returns the size of the the download.
func (d *Download) Size() int64 {
	return int64(d.size)
}
*/
// ChunkLocation returns the position in the file and the size of the chunk
func (d *Download) ChunkLocation(id int) (position int64, size int, err error) {
	if id < 0 || id >= len(d.chunks) {
		return 0, 0, ErrArgs
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.chunks[id].position, d.chunks[id].size, nil
}

// DownloadChunk gets a chunk with the given number and update the
// mac, returning the position in the file of the chunk
func (d *Download) DownloadChunk(id int) ([]byte, error) {
	if id < 0 || id >= len(d.chunks) {
		return nil, ErrArgs
	}

	// Local variables should be camelCase.
	chunkStart, chunkSize, err := d.ChunkLocation(id)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	chunkURL := fmt.Sprintf("%s/%d-%d", d.ResourceURL, chunkStart, chunkStart+int64(chunkSize)-1)
	sleepTime := minSleepTime // Initial backoff time
	for retry := 0; retry < d.m.retries+1; retry++ {
		resp, err = d.m.client.Get(chunkURL)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				break
			}
			err = errors.New("Http Status: " + resp.Status)
			_ = resp.Body.Close()
		}
		// If logger is external, it's fine. Otherwise, use standard Go logging or pass a logger.
		// log.Printf("%s: Retry download chunk %d/%d: %v", d.Name, retry, d.m.retries, err)
		backOffSleep(&sleepTime)
	}
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("retries exceeded")
	}

	chunk, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// body is read and closed here

	if len(chunk) != chunkSize {
		return nil, errors.New("wrong size for downloaded chunk")
	}

	// Decrypt the block
	ctrIV := make([]uint32, len(d.ivA32))
	copy(ctrIV, d.ivA32)
	ctrIV[2] = uint32(uint64(chunkStart) / 0x1000000000)
	ctrIV[3] = uint32(chunkStart / 0x10)
	bctrIV, err := a32ToBytes(ctrIV)
	if err != nil {
		return nil, err
	}
	ctrAES := cipher.NewCTR(d.aesBlock, bctrIV)
	ctrAES.XORKeyStream(chunk, chunk)
	// Update the chunk_macs
	enc := cipher.NewCBCEncrypter(d.aesBlock, d.iv)
	// Declaring `i` inside the for-loop is better for scope.
	block := make([]byte, 16)
	paddedChunk := paddNull(chunk, 16)
	for i := 0; i < len(paddedChunk); i += 16 {
		enc.CryptBlocks(block, paddedChunk[i:i+16])
	}

	d.mutex.Lock()
	if len(d.chunkMacs) > 0 {
		d.chunkMacs[id] = make([]byte, 16)
		copy(d.chunkMacs[id], block)
	}
	d.mutex.Unlock()

	return chunk, nil
}

// Finish checks the accumulated MAC for each block.
//
// If all the chunks weren't downloaded then it will just return nil.
func (d *Download) Finish() (err error) {
	// Can't check a 0 sized file
	if len(d.chunkMacs) == 0 {
		return nil
	}
	macData := make([]byte, 16)
	for _, v := range d.chunkMacs {
		// If a chunk_macs hasn't been set then the whole file
		// wasn't downloaded and we can't check it
		if v == nil {
			return nil
		}
		d.macEnc.CryptBlocks(macData, v)
	}

	tmac, err := bytesToA32(macData)
	if err != nil {
		return err
	}
	btmac, err := a32ToBytes([]uint32{tmac[0] ^ tmac[1], tmac[2] ^ tmac[3]})
	if err != nil {
		return err
	}
	metaMacTwo, err := a32ToBytes(d.metaMacTwo)
	if err != nil {
		return err
	}
	if !bytes.Equal(btmac, metaMacTwo) {
		return ErrMACMismatch
	}

	return nil
}
