package utils

import (
	"errors"
	"fmt"
	"strings"
	
	"github.com/gotd/td/tg"
	mtp_errors "github.com/celestix/gotgproto/errors"
	
	"enc/internal/bot/types"
)

type FileInfo struct {
	Name  string
	Size  int64
}

// mediaToFileMeta converts a tg.MessageMediaClass -> *FileMeta for documents/videos.
// Returns nil for non-document media.
func MediaToFileMeta(m tg.MessageMediaClass) *types.FileMeta {
	if m == nil {
		return nil
	}

	switch mm := m.(type) {
	case *tg.MessageMediaDocument:
		doc, _ := mm.Document.AsNotEmpty()
		if doc == nil {
			return nil
		}

		fileName := ""
		for _, attr := range doc.GetAttributes() {
			switch a := attr.(type) {
			case *tg.DocumentAttributeFilename:
				fileName = a.FileName
			}
			
		}

		// Mime & detection of video type
		mime := ""
		if mt := doc.GetMimeType(); mt != "" {
			mime = mt
		}

		typ := "document"
		if mime != "" && strings.HasPrefix(strings.ToLower(mime), "video/") {
			typ = "video"
		}

		// FileReference may be nil/empty
		var fileRef []byte
		if fr := doc.GetFileReference(); len(fr) > 0 {
			fileRef = make([]byte, len(fr))
			copy(fileRef, fr)
		}

		return &types.FileMeta{
			Type:       typ,
			DocumentID: doc.GetID(),
			AccessHash: doc.GetAccessHash(),
			FileRef:    fileRef,
			FileName:   fileName,
			MimeType:   mime,
			Size:       int64(doc.GetSize()),
		}

	default:
		// Not a document (photo, web page, etc.) — we ignore these by design.
		return nil
	}
}

// BuildInputFileLocation converts your FileMeta -> a tg.InputFileLocationClass.
// Returns nil + error if meta is nil or unsupported.
func BuildInputFileLocation(meta *types.FileMeta) (tg.InputFileLocationClass, error) {
	if meta == nil {
		return nil, errors.New("nil meta")
	}
	switch meta.Type {
	case "document", "video":
		return &tg.InputDocumentFileLocation{
			ID:            meta.DocumentID,
			AccessHash:    meta.AccessHash,
			FileReference: meta.FileRef,
			ThumbSize:     "", // empty = full file; set if you want a thumb
		}, nil
	default:
		return nil, fmt.Errorf("unsupported media type: %s", meta.Type)
	}
}


func GetInputFileLocationAndSize(media tg.MessageMediaClass) (tg.InputFileLocationClass, *FileInfo, error) {
	switch v := media.(type) {
	case *tg.MessageMediaPhoto: // messageMediaPhoto#695150d7
		f, ok := v.Photo.AsNotEmpty()
		if !ok {
			return nil, nil, mtp_errors.ErrUnknownTypeMedia
		}
		thumbSize := ""
		if len(f.Sizes) > 1 {
			// Lowest (f.Sizes[0]) size has the lowest resolution
			// Highest (f.Sizes[len(f.Sizes)-1]) has the highest resolution
			thumbSize = f.Sizes[len(f.Sizes)-1].GetType()
		}
		return &tg.InputPhotoFileLocation{
			ID:            f.ID,
			AccessHash:    f.AccessHash,
			FileReference: f.FileReference,
			ThumbSize:     thumbSize,
		}, nil, nil
	case *tg.MessageMediaDocument: // messageMediaDocument#4cf4d72d
		f, ok := v.Document.AsNotEmpty()
		if !ok {
			return nil, nil, mtp_errors.ErrUnknownTypeMedia
		}
		fileName := ""
		for _, attr := range f.GetAttributes() {
			switch a := attr.(type) {
			case *tg.DocumentAttributeFilename:
				fileName = a.FileName
			}
			
		}
		file := &FileInfo{
			Name: fileName,
			Size: f.Size,
		}
		return f.AsInputDocumentFileLocation(), file, nil
	}
	return nil, nil, mtp_errors.ErrUnknownTypeMedia
}