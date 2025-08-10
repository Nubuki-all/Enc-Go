package types

type FilterData struct {
	Fil1 string `bson:"fil1" json:"fil1"`
	Fil2 string `bson:"fil2" json:"fil2"`
	Fil3 string `bson:"fil3" json:"fil3"`
}

type RGExtendedFilterData struct {
	Match string               `bson:"match" json:"match"`
	Data  []*MatchedFilterData `bson:"data" json:"data"`
	CaseSensitive  bool   `bson:"casesensitive" json:"casesensitive"`
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
