// Package types contain bot specific inteenal types
package types

type CallbackInfo struct {
	*TransferInfo
	Process     *Process
	DlPath      string
	OutPath     string
	ReturnFlags string
}
