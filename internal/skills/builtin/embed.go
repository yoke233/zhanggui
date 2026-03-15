package builtin

import "embed"

//go:embed step-signal
var StepSignalFS embed.FS

//go:embed sys-step-manage
var SysStepManageFS embed.FS

//go:embed task-signal
var TaskSignalFS embed.FS

//go:embed all:*
var AllBuiltinFS embed.FS
