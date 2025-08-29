package types

import "os"

type CallbackInfo struct {
	*TransferInfo
	Process *os.Process
	DlName  string
	OutName string
}
