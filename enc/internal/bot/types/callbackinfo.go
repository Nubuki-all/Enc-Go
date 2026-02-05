// Package types contain bot specific internal types
package types

type CallbackInfo struct {
	*TransferInfo
	Process     *Process
	DlPath      string
	OutPath     string
	ReturnFlags string
}
