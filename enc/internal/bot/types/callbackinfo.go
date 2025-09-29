package types

import "os"

type CallbackInfo struct {
	*TransferInfo
	Process *os.Process
	DlPath  string
	OutPath string
}
