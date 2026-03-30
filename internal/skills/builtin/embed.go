package builtin

import "embed"

//go:embed action-signal
var ActionSignalFS embed.FS

//go:embed sys-action-manage
var SysActionManageFS embed.FS

//go:embed action-signal ceo-manage gstack-document-release gstack-office-hours gstack-plan-ceo-review gstack-plan-eng-review gstack-review plan-actions sys-action-manage
var AllBuiltinFS embed.FS
