package outbox

import (
	"context"
	"errors"

	domainoutbox "zhanggui/internal/domain/outbox"
	"zhanggui/internal/ports"
)

var (
	errActorRequired = errors.New("actor is required")
	errBodyRequired  = errors.New("body is required")
	errCloseEvidence = errors.New("close requires structured evidence with Changes and Tests")

	errIssueNotClaimed   = domainoutbox.ErrIssueNotClaimed
	errNeedsHuman        = domainoutbox.ErrNeedsHuman
	errDependsUnresolved = domainoutbox.ErrDependsUnresolved
	errTaskIssueBody     = domainoutbox.ErrTaskIssueBody
)

type Service struct {
	repo             ports.OutboxRepository
	uow              ports.UnitOfWork
	cache            ports.Cache
	workerInvoker    func(context.Context, invokeWorkerInput) error
	workResultLoader func(string) (WorkResultEnvelope, error)
	workdirFactory   func(workflowWorkdirConfig, string, string) (workdirManager, error)
	codexRunner      codexRunner
}

// NewService wires outbox usecases with repository and optional cache.
func NewService(repo ports.OutboxRepository, uow ports.UnitOfWork, cache ports.Cache) *Service {
	return &Service{
		repo:        repo,
		uow:         uow,
		cache:       cache,
		codexRunner: newDefaultCodexRunner(),
	}
}

type CreateIssueInput struct {
	Title  string
	Body   string
	Labels []string
}

type ClaimIssueInput struct {
	IssueRef string
	Assignee string
	Actor    string
	Comment  string
}

type CommentIssueInput struct {
	IssueRef string
	Actor    string
	Body     string
	State    string
}

type CloseIssueInput struct {
	IssueRef string
	Actor    string
	Comment  string
}

type UnclaimIssueInput struct {
	IssueRef string
	Actor    string
	Comment  string
}

type IngestQualityEventInput struct {
	IssueRef         string
	Source           string
	ExternalEventID  string
	Category         string
	Result           string
	Actor            string
	Summary          string
	Evidence         []string
	Payload          string
	ProvidedEventKey string
}

type IngestQualityEventResult struct {
	IssueRef         string
	IdempotencyKey   string
	Duplicate        bool
	CommentWritten   bool
	Marker           string
	RoutedRole       string
	NormalizedKind   string
	NormalizedResult string
}

type LeadRunIssueInput struct {
	Role           string
	Assignee       string
	IssueRef       string
	WorkflowFile   string
	ConfigFile     string
	ExecutablePath string
	ForceSpawn     bool
}

type LeadRunIssueResult struct {
	Processed bool
	Blocked   bool
	Spawned   bool
}

type WorkflowSummary struct {
	EnabledRoles []string
}

type IssueListItem struct {
	IssueRef  string
	Title     string
	Assignee  string
	IsClosed  bool
	CreatedAt string
	UpdatedAt string
	Labels    []string
}

type EventItem struct {
	EventID   uint64
	Actor     string
	CreatedAt string
	Body      string
}

type QualityEventItem struct {
	QualityEventID  uint64
	IdempotencyKey  string
	Source          string
	ExternalEventID string
	Category        string
	Result          string
	Actor           string
	Summary         string
	Evidence        []string
	PayloadJSON     string
	IngestedAt      string
}

type IssueDetail struct {
	IssueRef  string
	Title     string
	Body      string
	Assignee  string
	IsClosed  bool
	CreatedAt string
	UpdatedAt string
	ClosedAt  string
	Labels    []string
	Events    []EventItem
}

func (s *Service) setCacheBestEffort(ctx context.Context, key string, value string) {
	if s.cache == nil {
		return
	}
	_ = s.cache.Set(ctx, key, value, 0)
}
