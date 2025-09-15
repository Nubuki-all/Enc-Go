package mega

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func newHttpClient(timeout time.Duration) *http.Client {
	// TODO: Need to test this out
	// Doesn't seem to work as expected
	c := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				c, err := net.DialTimeout(netw, addr, timeout)
				if err != nil {
					return nil, err
				}
				return c, nil
			},
			Proxy: http.ProxyFromEnvironment,
		},
	}
	return c
}

// ParseMegaURL extracts id and key fragment from public file URL forms:
// https://mega.nz/file/<id>#<key> or legacy formats.
func parseMegaURL(u string) (id, keyFrag string, err error) {
	var parts []string
	if strings.Contains(u, "/file/") && strings.Contains(u, "#") {
		parts = strings.Split(u, "/file/")
		if len(parts) < 2 {
			return "", "", errors.New("invalid megalink")
		}
	} else if strings.Contains(u, "/folder/") && strings.Contains(u, "#") {
		parts = strings.Split(u, "/folder/")
		if len(parts) < 2 {
			return "", "", errors.New("invalid megalink")
		}
	}
	if len(parts) != 0 {
		rest := parts[1]
		id = strings.SplitN(rest, "#", 2)[0]
		keyFrag = strings.SplitN(rest, "#", 2)[1]
		return id, keyFrag, nil
	}
	// legacy #!id!key
	if strings.Contains(u, "#!") || strings.Contains(u, "!") {
		if idx := strings.LastIndex(u, "!"); idx != -1 {
			keyFrag = u[idx+1:]
			before := u[:idx]
			id = before[strings.LastIndexAny(before, "!/#")+1:]
			if id == "" || keyFrag == "" {
				return "", "", errors.New("couldn't parse legacy mega url")
			}
			return id, keyFrag, nil
		}
	}
	return "", "", EUNSUPPORTEDLINK
}

func isFolder (url) bool {
	return strings.Contains(url, "/folder/")
}

type urlInfo struct {
	ID string
	KeyFrag string
	Type int
	InnerFileHandle string
	InnerFolderHandle string
}

func parseMegaFolderURL(url string) (*urlInfo, error) {
	s := strings.Split(url, "mega.nz")
	if len(s) != 2 {
		return nil, EUNSUPPORTEDLINK
	}
	s = strings.Split(x[1], "/")
	if len(s) < 2 {
		return nil, EUNSUPPORTEDLINK
	}
	info := &urlInfo{}
	if strings.EqualFold(s[0], "folder") {
		info.Type = 1
	} else if strings.EqualFold(s[0], "file") {
		info.Type = 0
	}
	rest := s[1]
	info.ID = strings.SplitN(rest, "#", 2)[0]
	info.KeyFrag = strings.SplitN(rest, "#", 2)[1]
	if len(s) == 4 {
		if strings.EqualFold(s[2], "folder") {
			info.InnerFolderHandle = s[3]
		} else if strings.EqualFold(s[2], "file") {
			info.InnerFileHandle = s[3]
		}
	}
	return info, nil
}

// pruneNodesToPath keeps only the target node and its ancestors up to stopAt (inclusive).
// nodes must be a map[handle]FSNode as from the folder API. stopAt is typically the root handle parsed from the URL.
func pruneNodesToPath(nodes map[string]FSNode, targetHandle, stopAt string) (map[string]FSNode, map[string][]string, error) {
	if targetHandle == "" {
		return nil, nil, fmt.Errorf("empty target handle")
	}
	// verify target exists
	cur := targetHandle
	kept := make(map[string]FSNode)

	// Walk up parents until we reach stopAt or parent is empty.
	for {
		n, ok := nodes[cur]
		if !ok {
			return nil, nil, fmt.Errorf("handle not found in nodes: %s", cur)
		}
		kept[cur] = n
		// if we've reached the top stopAt OR we've reached a node whose parent is empty (share root), stop.
		if cur == stopAt || n.Parent == "" {
			break
		}
		// move to parent
		cur = n.Parent
	}

	// Build children map from kept nodes only (parent->[]children)
	children := make(map[string][]string)
	for h, n := range kept {
		// only link if parent is also in kept
		if n.Parent != "" {
			if _, ok := kept[n.Parent]; ok {
				children[n.Parent] = append(children[n.Parent], h)
			}
		}
	}

	return kept, children, nil
}


// bytesToA32 converts the byte slice b to uint32 slice considering
// the bytes to be in big endian order.
func bytesToA32(b []byte) ([]uint32, error) {
	length := len(b) + 3
	a := make([]uint32, length/4)
	buf := bytes.NewBuffer(b)
	for i, _ := range a {
		err := binary.Read(buf, binary.BigEndian, &a[i])
		if err != nil {
			return nil, err
		}
	}

	return a, nil
}

// a32ToBytes converts the uint32 slice a to byte slice where each
// uint32 is decoded in big endian order.
func a32ToBytes(a []uint32) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Grow(len(a) * 4) // To prevent reallocations in Write
	for _, v := range a {
		err := binary.Write(buf, binary.BigEndian, v)
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// decryptKeyA32 decrypts an encrypted 'k' a32 slice using the provided key a32 (folder key).
// It mirrors mega.py decrypt_key: AES-CBC decrypt each 16-byte block with key = a32_to_bytes(key) and zero IV.
func decryptKeyA32(encA32 []uint32, keyA32 []uint32) ([]uint32, error) {
	keyBytes := a32ToBytes(keyA32)
	encBytes := a32ToBytes(encA32) // converts encA32 to bytes
	// AES-CBC decrypt with zero IV
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}
	if len(encBytes)%aes.BlockSize != 0 {
		// pad (should not usually be needed)
		pad := aes.BlockSize - (len(encBytes) % aes.BlockSize)
		encBytes = append(encBytes, bytes.Repeat([]byte{0}, pad)...)
	}
	ivZero := make([]byte, aes.BlockSize)
	mode := cipher.NewCBCDecrypter(block, ivZero)
	dec := make([]byte, len(encBytes))
	mode.CryptBlocks(dec, encBytes)
	return bytesToA32(dec), nil
}

// base64urlencode encodes byte slice b using base64 url encoding
// without `=` padding.
func base64urlencode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// base64urldecode decodes the byte slice b using unpadded base64 url
// decoding. It also allows the characters from standard base64 to be
// compatible with the mega decoder.
func base64urldecode(s string) ([]byte, error) {
	enc := base64.RawURLEncoding
	// mega base64 decoder accepts the characters from both URLEncoding and StdEncoding
	// though nearly all strings are URL encoded
	s = strings.Replace(s, "+", "-", -1)
	s = strings.Replace(s, "/", "_", -1)
	return enc.DecodeString(s)
}

// base64ToA32 converts base64 encoded byte slice b to uint32 slice.
func base64ToA32(s string) ([]uint32, error) {
	d, err := base64urldecode(s)
	if err != nil {
		return nil, err
	}
	return bytesToA32(d)
}

// a32ToBase64 converts uint32 slice to base64 encoded byte slice.
func a32ToBase64(a []uint32) (string, error) {
	d, err := a32ToBytes(a)
	if err != nil {
		return "", err
	}
	return base64urlencode(d), nil
}

// paddNull pads byte slice b such that the size of resulting byte
// slice is a multiple of q.
func paddNull(b []byte, q int) []byte {
	if rem := len(b) % q; rem != 0 {
		l := q - rem

		for i := 0; i < l; i++ {
			b = append(b, 0)
		}
	}

	return b
}

// passwordKey calculates password hash from the user password.
func passwordKey(p string) ([]byte, error) {
	a, err := bytesToA32(paddNull([]byte(p), 4))
	if err != nil {
		return nil, err
	}

	pkey, err := a32ToBytes([]uint32{0x93C467E3, 0x7DB0C7A4, 0xD1BE3F81, 0x0152CB56})
	if err != nil {
		return nil, err
	}

	n := (len(a) + 3) / 4

	ciphers := make([]cipher.Block, n)

	for j := 0; j < len(a); j += 4 {
		key := []uint32{0, 0, 0, 0}
		for k := 0; k < 4; k++ {
			if j+k < len(a) {
				key[k] = a[k+j]
			}
		}
		bkey, err := a32ToBytes(key)
		if err != nil {
			return nil, err
		}
		ciphers[j/4], err = aes.NewCipher(bkey) // Uses AES in ECB mode
		if err != nil {
			return nil, err
		}
	}

	for i := 65536; i > 0; i-- {
		for j := 0; j < n; j++ {
			ciphers[j].Encrypt(pkey, pkey)
		}
	}

	return pkey, nil
}

// stringHash computes generic string hash. Uses k as the key for AES
// cipher.
func stringHash(s string, k []byte) (string, error) {
	a, err := bytesToA32(paddNull([]byte(s), 4))
	if err != nil {
		return "", err
	}
	h := []uint32{0, 0, 0, 0}
	for i, v := range a {
		h[i&3] ^= v
	}

	hb, err := a32ToBytes(h)
	if err != nil {
		return "", err
	}
	cipher, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	for i := 16384; i > 0; i-- {
		cipher.Encrypt(hb, hb)
	}
	ha, err := bytesToA32(paddNull(hb, 4))
	if err != nil {
		return "", err
	}

	return a32ToBase64([]uint32{ha[0], ha[2]})
}

// getMPI returns the length encoded Int and the next slice.
func getMPI(b []byte) (*big.Int, []byte) {
	p := new(big.Int)
	plen := (uint64(b[0])*256 + uint64(b[1]) + 7) >> 3
	p.SetBytes(b[2 : plen+2])
	b = b[plen+2:]
	return p, b
}

// getRSAKey decodes the RSA Key from the byte slice b.
func getRSAKey(b []byte) (*big.Int, *big.Int, *big.Int) {
	p, b := getMPI(b)
	q, b := getMPI(b)
	d, _ := getMPI(b)

	return p, q, d
}

// decryptRSA decrypts message m using RSA private key (p,q,d)
func decryptRSA(m, p, q, d *big.Int) []byte {
	n := new(big.Int)
	r := new(big.Int)
	n.Mul(p, q)
	r.Exp(m, d, n)

	return r.Bytes()
}

// blockDecrypt decrypts using the block cipher blk in ECB mode.
func blockDecrypt(blk cipher.Block, dst, src []byte) error {

	if len(src) > len(dst) || len(src)%blk.BlockSize() != 0 {
		return errors.New("Block decryption failed")
	}

	l := len(src) - blk.BlockSize()

	for i := 0; i <= l; i += blk.BlockSize() {
		blk.Decrypt(dst[i:], src[i:])
	}

	return nil
}

// blockEncrypt encrypts using the block cipher blk in ECB mode.
func blockEncrypt(blk cipher.Block, dst, src []byte) error {

	if len(src) > len(dst) || len(src)%blk.BlockSize() != 0 {
		return errors.New("Block encryption failed")
	}

	l := len(src) - blk.BlockSize()

	for i := 0; i <= l; i += blk.BlockSize() {
		blk.Encrypt(dst[i:], src[i:])
	}

	return nil
}

// decryptSeessionId decrypts the session id using the given private
// key.
func decryptSessionId(privk string, csid string, mk []byte) (string, error) {

	block, err := aes.NewCipher(mk)
	if err != nil {
		return "", err
	}
	pk, err := base64urldecode(privk)
	if err != nil {
		return "", err
	}
	err = blockDecrypt(block, pk, pk)
	if err != nil {
		return "", err
	}

	c, err := base64urldecode(csid)
	if err != nil {
		return "", err
	}

	m, _ := getMPI(c)

	p, q, d := getRSAKey(pk)
	r := decryptRSA(m, p, q, d)

	return base64urlencode(r[:43]), nil

}

// chunkSize describes a size and position of chunk
type chunkSize struct {
	position int64
	size     int
}

func getChunkSizes(size int64) (chunks []chunkSize) {
	p := int64(0)
	for i := 1; size > 0; i++ {
		var chunk int
		if i <= 8 {
			chunk = i * 131072
		} else {
			chunk = 1048576
		}
		if size < int64(chunk) {
			chunk = int(size)
		}
		chunks = append(chunks, chunkSize{position: p, size: chunk})
		p += int64(chunk)
		size -= int64(chunk)
	}
	return chunks
}

var attrMatch = regexp.MustCompile(`{".*"}`)

func decryptAttr(key []byte, data string) (attr FileAttr, err error) {
	err = EBADATTR
	block, err := aes.NewCipher(key)
	if err != nil {
		return attr, err
	}
	iv, err := a32ToBytes([]uint32{0, 0, 0, 0})
	if err != nil {
		return attr, err
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	buf := make([]byte, len(data))
	ddata, err := base64urldecode(data)
	if err != nil {
		return attr, err
	}
	mode.CryptBlocks(buf, ddata)

	if string(buf[:4]) == "MEGA" {
		str := strings.TrimRight(string(buf[4:]), "\x00")
		trimmed := attrMatch.FindString(str)
		if trimmed != "" {
			str = trimmed
		}
		err = json.Unmarshal([]byte(str), &attr)
	}
	return attr, err
}

func encryptAttr(key []byte, attr FileAttr) (b string, err error) {
	err = EBADATTR
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(attr)
	if err != nil {
		return "", err
	}
	attrib := []byte("MEGA")
	attrib = append(attrib, data...)
	attrib = paddNull(attrib, 16)

	iv, err := a32ToBytes([]uint32{0, 0, 0, 0})
	if err != nil {
		return "", err
	}
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(attrib, attrib)

	b = base64urlencode(attrib)
	return b, nil
}

func randString(l int) (string, error) {
	encoding := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/+"
	b := make([]byte, l)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	enc := base64.NewEncoding(encoding)
	d := make([]byte, enc.EncodedLen(len(b)))
	enc.Encode(d, b)
	d = d[:l]
	return strings.NewReplacer("/", "A", "+", "B").Replace(string(d)), nil
}