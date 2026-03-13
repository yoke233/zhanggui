package agentruntime

import (
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ThreadBootInput holds all context needed to assemble a boot prompt for an
// agent joining a Thread.
type ThreadBootInput struct {
	Thread         *core.Thread
	RecentMessages []*core.ThreadMessage
	Participants   []*core.ThreadParticipant
	WorkItems      []*core.WorkItem
	AgentProfile   *core.AgentProfile
	PriorSummary   string // progress_summary from a previous session (if resuming)
}

// BuildBootPrompt assembles a Markdown system prompt that orients an agent
// joining a Thread.  The prompt is intentionally concise to preserve prompt
// cache efficiency.
func BuildBootPrompt(in ThreadBootInput) string {
	var b strings.Builder

	// Custom template takes precedence.
	if in.AgentProfile != nil && strings.TrimSpace(in.AgentProfile.Session.ThreadBootTemplate) != "" {
		b.WriteString(strings.TrimSpace(in.AgentProfile.Session.ThreadBootTemplate))
		b.WriteString("\n\n")
	}

	// Thread context.
	b.WriteString("## Thread Context\n")
	if in.Thread != nil {
		fmt.Fprintf(&b, "**Title**: %s\n", in.Thread.Title)
		fmt.Fprintf(&b, "**Status**: %s\n", in.Thread.Status)
	}
	if in.AgentProfile != nil {
		fmt.Fprintf(&b, "**Your Role**: %s (%s)\n", in.AgentProfile.Role, in.AgentProfile.ID)
	}
	b.WriteString("\n")

	// Recent conversation.
	if len(in.RecentMessages) > 0 {
		fmt.Fprintf(&b, "## Recent Conversation (last %d messages)\n", len(in.RecentMessages))
		for _, msg := range in.RecentMessages {
			sender := msg.SenderID
			if sender == "" {
				sender = "anonymous"
			}
			fmt.Fprintf(&b, "[%s] (%s): %s\n", sender, msg.Role, msg.Content)
		}
		b.WriteString("\n")
	}

	// Participants.
	if len(in.Participants) > 0 {
		b.WriteString("## Participants\n")
		for _, p := range in.Participants {
			marker := ""
			if in.AgentProfile != nil && p.UserID == in.AgentProfile.ID {
				marker = " ← you"
			}
			fmt.Fprintf(&b, "- %s (%s)%s\n", p.UserID, p.Role, marker)
		}
		b.WriteString("\n")
	}

	// Linked work items.
	if len(in.WorkItems) > 0 {
		b.WriteString("## Linked Work Items\n")
		for _, wi := range in.WorkItems {
			fmt.Fprintf(&b, "- #%d: %s [status: %s]\n", wi.ID, wi.Title, wi.Status)
		}
		b.WriteString("\n")
	}

	// Prior context (resuming from a paused session).
	if strings.TrimSpace(in.PriorSummary) != "" {
		b.WriteString("## Prior Context (resuming)\n")
		b.WriteString(strings.TrimSpace(in.PriorSummary))
		b.WriteString("\n\n")
	}

	// Instructions.
	b.WriteString("## Instructions\n")
	b.WriteString("You are joining this thread. Review the context above and respond when addressed.\n")

	return b.String()
}
