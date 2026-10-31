package tele

// minimal: decode bot file_id -> media_id, access_hash, file_ref, dc
import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"

	"enc/internal/clients/telegram"

	"github.com/gotd/td/tg"
)

const (
	webLocationFlag = 1 << 24
	fileRefFlag     = 1 << 25
)

// rle decode used by file_id
func rleDecode(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		b := in[i]
		if b == 0 {
			i++
			if i >= len(in) {
				break
			}
			n := int(in[i])
			for j := 0; j < n; j++ {
				out = append(out, 0)
			}
		} else {
			out = append(out, b)
		}
	}
	return out
}

// TL Bytes read (Telegram's "Bytes" type)
func readTLBytes(r *bytes.Reader) ([]byte, error) {
	h, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	var n int
	if h < 254 {
		n = int(h)
	} else {
		// 3 bytes little endian length
		lenBytes := make([]byte, 3)
		if _, err := r.Read(lenBytes); err != nil {
			return nil, err
		}
		n = int(lenBytes[0]) | int(lenBytes[1])<<8 | int(lenBytes[2])<<16
	}
	data := make([]byte, n)
	if _, err := r.Read(data); err != nil {
		return nil, err
	}
	// padding to 4-byte boundary
	pad := (4 - ((n + 1) % 4)) % 4
	if h >= 254 {
		// header was 4 bytes total (0xFE + 3), padding uses (n % 4)
		pad = (4 - (n % 4)) % 4
	}
	if pad > 0 {
		if _, err := r.Seek(int64(pad), 1); err != nil {
			return nil, err
		}
	}
	return data, nil
}

type DecodedFileID struct {
	DCID       int32
	FileType   int32
	MediaID    int64
	AccessHash int64
	FileRef    []byte // optional
}

// DecodeBotFileID decodes a bot file_id into relevant fields
func DecodeBotFileID(fileID string) (*DecodedFileID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(fileID)
	if err != nil {
		return nil, err
	}
	dec := rleDecode(raw)
	if len(dec) < 9 {
		return nil, errors.New("fileid too short")
	}
	major := dec[len(dec)-1]
	var buf []byte
	if major < 4 {
		buf = dec[:len(dec)-1]
	} else {
		// has minor byte
		if len(dec) < 10 {
			return nil, errors.New("fileid too short for v4")
		}
		buf = dec[:len(dec)-2]
	}
	r := bytes.NewReader(buf)
	var fileType int32
	var dcID int32
	if err := binary.Read(r, binary.LittleEndian, &fileType); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &dcID); err != nil {
		return nil, err
	}
	hasWeb := (fileType & webLocationFlag) != 0
	hasRef := (fileType & fileRefFlag) != 0
	fileType &= ^webLocationFlag
	fileType &= ^fileRefFlag

	if hasWeb {
		// not typical for stickers; bail or parse TL String+int64 if needed
		return nil, errors.New("web location not handled")
	}

	var fileRef []byte
	if hasRef {
		fb, err := readTLBytes(r)
		if err != nil {
			return nil, err
		}
		fileRef = fb
	}

	var mediaID int64
	var accessHash int64
	if err := binary.Read(r, binary.LittleEndian, &mediaID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &accessHash); err != nil {
		return nil, err
	}

	return &DecodedFileID{
		DCID:       dcID,
		FileType:   fileType,
		MediaID:    mediaID,
		AccessHash: accessHash,
		FileRef:    fileRef,
	}, nil
}

func SendStickerByFileID(chatID int64, botFileID string) (_ *Message, err error) {
	info, err := DecodeBotFileID(botFileID)
	if err != nil {
		return
	}

	loc := &tg.InputDocument{
		ID:            info.MediaID,
		AccessHash:    info.AccessHash,
		FileReference: info.FileRef, // nil if absent
	}

	media := &tg.InputMediaDocument{ID: loc}
	ctx := telegram.Context
	msg, err := ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{Media: media})
	if err != nil {
		return
	}
	return NewMsgFromGotg(msg, ctx, chatID, ctx.Self.ID), err
}
