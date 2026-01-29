package agui

// ThreadStatusSource 是可选扩展接口：用于让 run 在 step 边界读取 Thread 控制状态（例如 PAUSE_REQUESTED）。
//
// 说明：agui 包不依赖具体的“Thread 存储/服务”实现；因此扩展接口只暴露最小字段（string），存储细节由实现方决定。
type ThreadStatusSource interface {
	ThreadStatus(threadID string) (phase string, lastChangeSetID string, err error)
}

// ChangeSetApplier 是可选扩展接口：用于在 resume/intake 阶段记录用户决策并推进 ChangeSet 生命周期。
type ChangeSetApplier interface {
	ApplyChangeSet(threadID string, runID string, changeSetID string, decision any) error
}
