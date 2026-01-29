package taskbundle

type LatestPointer struct {
	SchemaVersion int         `json:"schema_version"`
	TaskID        string      `json:"task_id"`
	PackID        string      `json:"pack_id"`
	Rev           string      `json:"rev"`
	CreatedAt     string      `json:"created_at"`
	Paths         LatestPaths `json:"paths"`
}

type LatestPaths struct {
	BundleRoot   string `json:"bundle_root"`
	EvidenceZip  string `json:"evidence_zip"`
	ArtifactsZip string `json:"artifacts_zip"`
}

type LedgerActor struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Role string `json:"role"`
}

type Correlation struct {
	BundleID string `json:"bundle_id"`

	ThreadID string `json:"thread_id,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Rev      string `json:"rev,omitempty"`
	IntentID string `json:"intent_id,omitempty"`
	PackID   string `json:"pack_id,omitempty"`
}

type Trace struct {
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

type Ref struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Path    string `json:"path"`
	Sha256  string `json:"sha256,omitempty"`
	Size    *int64 `json:"size,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type LedgerEvent struct {
	SchemaVersion int         `json:"schema_version"`
	TS            string      `json:"ts"`
	Seq           uint64      `json:"seq"`
	EventID       string      `json:"event_id"`
	EventType     string      `json:"event_type"`
	Actor         LedgerActor `json:"actor"`
	Correlation   Correlation `json:"correlation"`
	Refs          []Ref       `json:"refs"`
	Payload       any         `json:"payload"`
	Trace         *Trace      `json:"trace,omitempty"`
}

type VerifyReport struct {
	SchemaVersion int            `json:"schema_version"`
	Correlation   Correlation    `json:"correlation"`
	Criteria      CriteriaRef    `json:"criteria"`
	Results       []ReportResult `json:"results"`
	Summary       ReportSummary  `json:"summary"`
}

type CriteriaRef struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Sha256  string `json:"sha256"`
	Path    string `json:"path"`
}

type ReportResult struct {
	CriteriaID   string `json:"criteria_id"`
	Status       string `json:"status"`
	Severity     string `json:"severity"`
	EvidenceRefs []Ref  `json:"evidence_refs,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

type ReportSummary struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Blocker int `json:"blocker"`
}
