package builtin

import "embed"

//go:embed action-signal
var ActionSignalFS embed.FS

//go:embed sys-action-manage
var SysActionManageFS embed.FS

//go:embed task-signal
var TaskSignalFS embed.FS

//go:embed all:*
var AllBuiltinFS embed.FS
