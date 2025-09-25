package types

import (
    "database/sql/driver"
    "encoding/json"
    "fmt"
)

type FilterData struct {
	Fil1 string `bson:"fil1" json:"fil1"`
	Fil2 string `bson:"fil2" json:"fil2"`
	Fil3 string `bson:"fil3" json:"fil3"`
}

type RGExtendedFilterData struct {
	Match         string               `bson:"match" json:"match"`
	Data          []*MatchedFilterData `bson:"data" json:"data"`
	CaseSensitive bool                 `bson:"casesensitive" json:"casesensitive"`
}

type MatchedFilterData struct {
	Match         string `bson:"match" json:"match"`
	FileTag       string `bson:"filetag" json:"filetag"`
	CaptionTag    string `bson:"captiontag" json:"captiontag"`
	CaseSensitive bool   `bson:"casesensitive" json:"casesensitive"`
	MatchOnce     bool   `bson:"matchonce" json:"matchonce"`
}

type RenameFilterData struct {
	Match          string `bson:"match" json:"match"`
	ReplaceName    string `bson:"replacename" json:"replacename"`
	ReplaceCaption string `bson:"replacecaption" json:"replacecaption"`
	ReplaceBanner  string `bson:"replacebanner" json:"replacebanner"`
	CaseSensitive  bool   `bson:"casesensitive" json:"casesensitive"`
	DisableAnilist bool   `bson:"disableanilist" json:"disableanilist"`
}

// Value implements driver.Valuer — converts FilterData -> JSON string for DB.
func (f FilterData) Value() (driver.Value, error) {
    // if you want to store NULL when zero value:
    // if reflect.DeepEqual(f, FilterData{}) { return nil, nil }
    b, err := json.Marshal(f)
    if err != nil {
        return nil, fmt.Errorf("marshal FilterData: %w", err)
    }
    return string(b), nil // or []byte(b)
}

// Scan implements sql.Scanner — converts DB value -> FilterData.
func (f *FilterData) Scan(src any) error {
    if src == nil {
        *f = FilterData{}
        return nil
    }
    var data []byte
    switch v := src.(type) {
    case []byte:
        data = v
    case string:
        data = []byte(v)
    default:
        return fmt.Errorf("unsupported Scan source type %T", src)
    }
    if len(data) == 0 {
        *f = FilterData{}
        return nil
    }
    if err := json.Unmarshal(data, f); err != nil {
        return fmt.Errorf("unmarshal FilterData: %w", err)
    }
    return nil
}
