package mega

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	mrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
	
	"enc/internal/logger"
)

// from https://github.com/t3rm1n4l/go-mega/blob/master/mega.go#L55

const (
	API_URL              = "https://g.api.mega.co.nz"
	BASE_DOWNLOAD_URL    = "https://mega.co.nz"
	RETRIES              = 10
	DOWNLOAD_WORKERS     = 3
	MAX_DOWNLOAD_WORKERS = 30
	UPLOAD_WORKERS       = 1
	MAX_UPLOAD_WORKERS   = 30
	TIMEOUT              = time.Second * 10
	HTTPSONLY            = false
	minSleepTime         = 10 * time.Millisecond // for retries
	maxSleepTime         = 5 * time.Second       // for retries
)

type config struct {
	baseurl    string
	retries    int
	dl_workers int
	ul_workers int
	timeout    time.Duration
	https      bool
}

func newConfig() config {
	return config{
		baseurl:    API_URL,
		retries:    RETRIES,
		dl_workers: DOWNLOAD_WORKERS,
		ul_workers: UPLOAD_WORKERS,
		timeout:    TIMEOUT,
		https:      HTTPSONLY,
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
	mgfs := newMegaFS()
	m := &Mega{
		config: cfg,
		/*sn:     bigx.Int64(),
		FS:     mgfs,*/
		client: newHttpClient(cfg.timeout),
	}
	return m
}

// API request method
func (m *Mega) apiRequest(r []byte, folderID string) (buf []byte, err error) {
	var resp *http.Response
	// serialize the API requests
	m.apiMu.Lock()
	defer func() {
		m.sn++
		m.apiMu.Unlock()
	}()

	url := m.baseurl + "/cs"
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
		buf, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			continue
		}
		err = resp.Body.Close()
		if err != nil {
			continue
		}

		// at this point the body is read and closed

		if bytes.HasPrefix(buf, []byte("[")) == false && bytes.HasPrefix(buf, []byte("-")) == false {
			return nil, EBADRESP
		}

		if len(buf) < 6 {
			var emsg [1]ErrorMsg
			err = json.Unmarshal(buf, &emsg)
			if err != nil {
				err = json.Unmarshal(buf, &emsg[0])
			}
			if err != nil {
				return buf, EBADRESP
			}
			err = parseError(emsg[0])
			if err == EAGAIN {
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

type Download struct {
	m           *Mega
	//src         *Node
	resourceUrl string
	name        string
	size        uint64
	aes_block   cipher.Block
	iv          []byte
	ivA32       []uint32
	mac_enc     cipher.BlockMode
	metaMacTwo  []uint32
	mutex       sync.Mutex // to protect the following
	chunks      []chunkSize
	chunk_macs  [][]byte
}

// an all nil IV for mac calculations
var zero_iv = make([]byte, 16)


func (m *Mega) NewDownloadsFromFolderLink(url string) ([]*Download, error) {
	if url == "" {
		return nil, EARGS
	}
	folderInfo, err := parseMegaURL(url)
	if err != nil {
		return nil, err
	}
	if folderInfo.Type == 0{
		return nil, fmt.Errorf("not a folder link")
	}
	folderKeyA32, err := base64ToA32(folderInfo.KeyFrag)
	if err != nil {
		return nil, fmt.Errorf("decode key fragment: %w", err)
	}
	
	var msg [1]DownloadFolderMsg
	var res [1]DownloadFolderResp

	m.FS.mutex.Lock()
	msg[0].Cmd = "f"
	msg[0].C = 1
	msg[0].CA = 1
	msg[0].R = 1
	
	key := kBytes
	m.FS.mutex.Unlock()
	
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
	for _, node := range res[0].F {
		
	}
	
	nodes := map[string]FSNode{}
	children := map[string][]string{}
	for _, n := range folderResp[0].F {
		nodes[n.Hash] = n
		children[n.Parent] = append(children[n.Parent], n.Hash)
	}
	pathHandle := folderInfo.InnerFolderHandle
	sFileHandle := folderInfo.InnerFileHandle
	startHandle := folderInfo.ID
	startKey := folderKeyA32
	if sFileHandle != "" {
	    prunedNodes, prunedChildren, perr := pruneNodesToPath(nodes, sFileHandle, rootHandle)
	    if perr != nil {
	        // decide behaviour: error out or fallback to full-tree behavior
	        return nil, fmt.Errorf("prune path: %w", perr)
	    }
	    // replace nodes & children with pruned versions
	    nodes = prunedNodes
	    children = prunedChildren
	    // now continue — your existing recursive walk (starting at rootHandle) will only see the ancestor chain
	}
	if pathHandle != "" {
		curKey := folderKeyA32
		curHandle := rootHandle
		if node, ok := nodes[pathHandle]; ok {
			parts := strings.Split(node.Key, ":")
			encB64 := parts[len(parts)-1]
			encA32, derr := base64ToA32(encB64)
			if derr != nil {
				return nil, fmt.Errorf("failed to parse child folder key for %s: %w", ph, derr)
			}
			childKeyA32, derr := decryptKeyA32(encA32, curKey)
			if derr != nil {
				return nil, fmt.Errorf("failed to decrypt child folder key for %s: %w", ph, derr)
			}
			// advance
			curKey = childKeyA32
			curHandle = ph
		}
		startHandle = curHandle
		startKey = curKey
	}
	
	zeroIV := make([]byte, 16)

	var downloads []*Download
	walk := func(curHandle string, curKeyA32 []uint32, relPath string) error {
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
			decA32, derr := decryptKeyA32(encA32, curKeyA32)
			if derr != nil {
				logger.Warn("skipped 1;  cannot decrypt with current folder key; " + derr.Error())
				continue
			}

			// if folder -> recurse using the decrypted child folder key (decA32)
			if node.T == 1 {
				// try to obtain a friendly folder name from attrs (if present)
				fName := node.Hash
				if node.Attr != "" && len(decA32) >= 8 {
					// derive short kBytes to decrypt attrs like file keys do
					kA32 := make([]uint32, 4)
					for i := 0; i < 4; i++ {
						kA32[i] = decA32[i] ^ decA32[i+4]
					}
					kBytes := a32ToBytes(kA32)
					if attrs, derr2 := decryptAttr(kBytes, node.Attr); derr2 == nil {
						if nname := attrs.Name; nname != "" {
							fName = nname
						}
					}
				}
				newRel := filepath.Join(relPath, filepath.Base(fName))
				if err := walk(node.Hash, decA32, newRel); err != nil {
					return err
				}
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
			keyBytes := a32ToBytes(kA32)

			// build AES block and MAC encrypter
			aesBlock, err := aes.NewCipher(keyBytes)
			if err != nil {
				return err
			}
			macEnc := cipher.NewCBCEncrypter(aesBlock, zeroIV)

			// iv for CTR is a32_to_bytes([iv0, iv1, iv0, iv1])
			ivBytes, err := a32_to_bytes([]uint32{decA32[4], decA32[5], decA32[4], decA32[5]})
			if err != nil {
				return err
			}
			var msg [1]DownloadMsg
			var res [1]DownloadResp
		
			m.FS.mutex.Lock()
			msg[0].Cmd = "g"
			msg[0].G = 1
			msg[0].P = node.Hash
			if m.config.https {
				msg[0].SSL = 2
			}
			m.FS.mutex.Unlock()

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
			if len(res) == 0 {
				continue
			}
			// DownloadResp has an embedded error in it for some reason
			if res[0].Err != 0 {
				logger.Warn(parseError(res[0].Err))
			}

			// decrypt attributes from API response if present to get filename
			fileName := node.Hash + ".bin"
			if gres[0].Attr != "" {
				if attrObj, derr := decryptAttr(keyBytes, gres[0].Attr); derr == nil {
					if attrObj.Name != "" {
						fileName = attrObj.Name
					}
				}
			} else if node.Attr != "" {
				// try folder-provided attr as fallback
				if attrObj, derr := decryptAttr(keyBytes, node.Attr); derr == nil {
					if attrObj.Name != "" {
						fileName = attrObj.Name
					}
				}
			}

			// build chunks and download URL
			downloadUrl := gres[0].G
			if m.config.https && strings.HasPrefix(downloadUrl, "http://") {
				downloadUrl = "https://" + strings.TrimPrefix(downloadUrl, "http://")
			}
			chunks := getChunkSizes(int64(gres[0].Size))

			// build Download and set name to include parent path (relPath)
			relName := fileName
			if relPath != "" {
				relName = filepath.Join(relPath, fileName)
			}
			d := &Download{
				m:           m,
				resourceUrl: downloadUrl,
				name:        relName,
				size:        gres[0].Size,
				aes_block:   aesBlock,
				iv:          ivBytes,
				ivA32:       []uint32{decA32[4], decA32[5]},
				mac_enc:     macEnc,
				metaMacTwo:  []uint32{decA32[6], decA32[7]},
				chunks:      chunks,
				chunk_macs:  make([][]byte, len(chunks)),
			}
			downloads = append(downloads, d)
		}
		return nil
	}
	// begin walk from the chosen startHandle using startKey (derived above)
	if err := walk(startHandle, startKey, ""); err != nil {
		return nil, err
	}
	if len(downloads) == 0 {
		return nil, errors.New("no downloadable files found (or unable to decrypt keys)")
	}
	return downloads, nil
}
// Create a new Download from link
//
// Call Chunks to find out how many chunks there are, then for id =
// 0..chunks-1 call DownloadChunk.  Finally call Finish() to receive
// the error status.
func (m *Mega) NewDownloadFromLink(url string) ([]*Download, error) {
	if url == "" {
		return nil, EARGS
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
	kBytes := a32ToBytes(kA32)
	var msg [1]DownloadMsg
	var res [1]DownloadResp

	m.FS.mutex.Lock()
	msg[0].Cmd = "g"
	msg[0].G = 1
	msg[0].P = id
	if m.config.https {
		msg[0].SSL = 2
	}
	key := kBytes
	m.FS.mutex.Unlock()

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

	aes_block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	mac_enc := cipher.NewCBCEncrypter(aes_block, zero_iv)
	m.FS.mutex.Lock()
	// t, err := bytes_to_a32(src.meta.iv)
	m.FS.mutex.Unlock()
	if err != nil {
		return nil, err
	}
	iv, err := a32_to_bytes([]uint32{ivA32[0], ivA32[1], ivA32[0], ivA32[1]})
	if err != nil {
		return nil, err
	}
	downloadUrl := res[0].G
	if m.config.https && strings.HasPrefix(downloadUrl, "http://") {
		downloadUrl = "https://" + strings.TrimPrefix(downloadUrl, "http://")
	}

	d := &Download{
		m:           m,
		//src:         src,
		resourceUrl: downloadUrl,
		name:        attr.Name,
		size:        res[0].Size
		aes_block:   aes_block,
		iv:          iv,
		ivA32:       ivA32,
		mac_enc:     mac_enc,
		metaMacTwo:  metaMacTwo,
		chunks:      chunks,
		chunk_macs:  make([][]byte, len(chunks)),
	}
	return []*Download{d}, nil
}

// Chunks returns The number of chunks in the download.
func (d *Download) Chunks() int {
	return len(d.chunks)
}

// Name returns the file name of the the download.
func (d *Download) Name() string {
	return d.name
}

// Size returns the size of the the download.
func (d *Download) Size() int64 {
	return int64(d.size)
}

// ChunkLocation returns the position in the file and the size of the chunk
func (d *Download) ChunkLocation(id int) (position int64, size int, err error) {
	if id < 0 || id >= len(d.chunks) {
		return 0, 0, EARGS
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.chunks[id].position, d.chunks[id].size, nil
}

// DownloadChunk gets a chunk with the given number and update the
// mac, returning the position in the file of the chunk
func (d *Download) DownloadChunk(id int) (chunk []byte, err error) {
	if id < 0 || id >= len(d.chunks) {
		return nil, EARGS
	}

	chk_start, chk_size, err := d.ChunkLocation(id)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	chunk_url := fmt.Sprintf("%s/%d-%d", d.resourceUrl, chk_start, chk_start+int64(chk_size)-1)
	sleepTime := minSleepTime // inital backoff time
	for retry := 0; retry < d.m.retries+1; retry++ {
		resp, err = d.m.client.Get(chunk_url)
		if err == nil {
			if resp.StatusCode == 200 {
				break
			}
			err = errors.New("Http Status: " + resp.Status)
			_ = resp.Body.Close()
		}
		logger.Info("%s: Retry download chunk %d/%d: %v", d.name, retry, d.m.retries, err)
		backOffSleep(&sleepTime)
	}
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("retries exceeded")
	}

	chunk, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// body is read and closed here

	if len(chunk) != chk_size {
		return nil, errors.New("wrong size for downloaded chunk")
	}

	// Decrypt the block
	ctr_iv := make([]uint32, len(d.ivA32))
	copy(ctr_iv, d.ivA32)
	ctr_iv[2] = uint32(uint64(chk_start) / 0x1000000000)
	ctr_iv[3] = uint32(chk_start / 0x10)
	bctr_iv, err := a32ToBytes(ctr_iv)
	if err != nil {
		return nil, err
	}
	ctr_aes := cipher.NewCTR(d.aes_block, bctr_iv)
	ctr_aes.XORKeyStream(chunk, chunk)
	// Update the chunk_macs
	enc := cipher.NewCBCEncrypter(d.aes_block, d.iv)
	i := 0
	block := make([]byte, 16)
	paddedChunk := paddnull(chunk, 16)
	for i = 0; i < len(paddedChunk); i += 16 {
		enc.CryptBlocks(block, paddedChunk[i:i+16])
	}

	d.mutex.Lock()
	if len(d.chunk_macs) > 0 {
		d.chunk_macs[id] = make([]byte, 16)
		copy(d.chunk_macs[id], block)
	}
	d.mutex.Unlock()

	return chunk, nil
}

// Finish checks the accumulated MAC for each block.
//
// If all the chunks weren't downloaded then it will just return nil
func (d *Download) Finish() (err error) {
	// Can't check a 0 sized file
	if len(d.chunk_macs) == 0 {
		return nil
	}
	mac_data := make([]byte, 16)
	for _, v := range d.chunk_macs {
		// If a chunk_macs hasn't been set then the whole file
		// wasn't downloaded and we can't check it
		if v == nil {
			return nil
		}
		d.mac_enc.CryptBlocks(mac_data, v)
	}

	tmac, err := bytesToA32(mac_data)
	if err != nil {
		return err
	}
	btmac, err := a32ToBytes([]uint32{tmac[0] ^ tmac[1], tmac[2] ^ tmac[3]})
	if err != nil {
		return err
	}
	if bytes.Equal(btmac, d.metaMacTwo) == false {
		return EMACMISMATCH
	}

	return nil
}

