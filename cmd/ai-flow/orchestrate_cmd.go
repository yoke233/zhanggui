package main

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newOrchestrateCmd(deps commandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestrate",
		Short: "Run orchestration control actions",
	}
	cmd.AddCommand(newOrchestrateTaskCmd(deps))
	return cmd
}

func newOrchestrateTaskCmd(deps commandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Run task orchestration actions",
	}
	cmd.AddCommand(
		newOrchestrateTaskCreateCmd(deps),
		newOrchestrateTaskFollowUpCmd(deps),
		newOrchestrateTaskAssignProfileCmd(deps),
		newOrchestrateTaskReassignCmd(deps),
		newOrchestrateTaskDecomposeCmd(deps),
		newOrchestrateTaskEscalateThreadCmd(deps),
	)
	return cmd
}

func newOrchestrateTaskCreateCmd(deps commandDeps) *cobra.Command {
	var title string
	var body string
	var projectID int64
	var priority string
	var labels string
	var dedupeKey string
	var sourceGoalRef string
	var sourceSession string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an orchestrated task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := []string{"task", "create"}
			if cmd.Flags().Changed("title") {
				forwardArgs = append(forwardArgs, "--title", title)
			}
			if cmd.Flags().Changed("body") {
				forwardArgs = append(forwardArgs, "--body", body)
			}
			if cmd.Flags().Changed("project-id") {
				forwardArgs = append(forwardArgs, "--project-id", strconv.FormatInt(projectID, 10))
			}
			if cmd.Flags().Changed("priority") {
				forwardArgs = append(forwardArgs, "--priority", priority)
			}
			if cmd.Flags().Changed("labels") {
				forwardArgs = append(forwardArgs, "--labels", labels)
			}
			if cmd.Flags().Changed("dedupe-key") {
				forwardArgs = append(forwardArgs, "--dedupe-key", dedupeKey)
			}
			if cmd.Flags().Changed("source-goal-ref") {
				forwardArgs = append(forwardArgs, "--source-goal-ref", sourceGoalRef)
			}
			if cmd.Flags().Changed("source-session") {
				forwardArgs = append(forwardArgs, "--source-session", sourceSession)
			}
			if cmd.Flags().Changed("json") && jsonOutput {
				forwardArgs = append(forwardArgs, "--json")
			}
			return deps.runOrchestrate(forwardArgs)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Task title")
	cmd.Flags().StringVar(&body, "body", "", "Task body")
	cmd.Flags().Int64Var(&projectID, "project-id", 0, "Project ID")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority")
	cmd.Flags().StringVar(&labels, "labels", "", "Comma-separated labels")
	cmd.Flags().StringVar(&dedupeKey, "dedupe-key", "", "Dedupe key for chat goal reuse")
	cmd.Flags().StringVar(&sourceGoalRef, "source-goal-ref", "", "Source goal reference")
	cmd.Flags().StringVar(&sourceSession, "source-session", "", "Source chat session ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	return cmd
}

func newOrchestrateTaskFollowUpCmd(deps commandDeps) *cobra.Command {
	var workItemID int64
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "follow-up",
		Short: "Follow up an orchestrated task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := []string{"task", "follow-up"}
			if cmd.Flags().Changed("work-item-id") {
				forwardArgs = append(forwardArgs, "--work-item-id", strconv.FormatInt(workItemID, 10))
			}
			if cmd.Flags().Changed("json") && jsonOutput {
				forwardArgs = append(forwardArgs, "--json")
			}
			return deps.runOrchestrate(forwardArgs)
		},
	}
	cmd.Flags().Int64Var(&workItemID, "work-item-id", 0, "Work item ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	return cmd
}

func newOrchestrateTaskAssignProfileCmd(deps commandDeps) *cobra.Command {
	var workItemID int64
	var profile string
	var reason string
	var actorProfile string
	var sourceSession string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "assign-profile",
		Short: "Assign the preferred profile for a task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := []string{"task", "assign-profile"}
			appendReassignFlags(cmd, &forwardArgs, workItemID, profile, reason, actorProfile, sourceSession, jsonOutput)
			return deps.runOrchestrate(forwardArgs)
		},
	}
	cmd.Flags().Int64Var(&workItemID, "work-item-id", 0, "Work item ID")
	cmd.Flags().StringVar(&profile, "profile", "", "Preferred profile ID")
	cmd.Flags().StringVar(&reason, "reason", "", "Assignment reason")
	cmd.Flags().StringVar(&actorProfile, "actor-profile", "", "Actor profile ID")
	cmd.Flags().StringVar(&sourceSession, "source-session", "", "Source chat session ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	return cmd
}

func newOrchestrateTaskReassignCmd(deps commandDeps) *cobra.Command {
	var workItemID int64
	var profile string
	var reason string
	var actorProfile string
	var sourceSession string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "reassign",
		Short: "Reassign a task to another profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := []string{"task", "reassign"}
			appendReassignFlags(cmd, &forwardArgs, workItemID, profile, reason, actorProfile, sourceSession, jsonOutput)
			return deps.runOrchestrate(forwardArgs)
		},
	}
	cmd.Flags().Int64Var(&workItemID, "work-item-id", 0, "Work item ID")
	cmd.Flags().StringVar(&profile, "profile", "", "New profile ID")
	cmd.Flags().StringVar(&reason, "reason", "", "Reassignment reason")
	cmd.Flags().StringVar(&actorProfile, "actor-profile", "", "Actor profile ID")
	cmd.Flags().StringVar(&sourceSession, "source-session", "", "Source chat session ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	return cmd
}

func newOrchestrateTaskDecomposeCmd(deps commandDeps) *cobra.Command {
	var workItemID int64
	var objective string
	var overwriteExisting bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "decompose",
		Short: "Generate execution actions for a task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := []string{"task", "decompose"}
			if cmd.Flags().Changed("work-item-id") {
				forwardArgs = append(forwardArgs, "--work-item-id", strconv.FormatInt(workItemID, 10))
			}
			if cmd.Flags().Changed("objective") {
				forwardArgs = append(forwardArgs, "--objective", objective)
			}
			if cmd.Flags().Changed("overwrite-existing") && overwriteExisting {
				forwardArgs = append(forwardArgs, "--overwrite-existing")
			}
			if cmd.Flags().Changed("json") && jsonOutput {
				forwardArgs = append(forwardArgs, "--json")
			}
			return deps.runOrchestrate(forwardArgs)
		},
	}
	cmd.Flags().Int64Var(&workItemID, "work-item-id", 0, "Work item ID")
	cmd.Flags().StringVar(&objective, "objective", "", "Decomposition objective")
	cmd.Flags().BoolVar(&overwriteExisting, "overwrite-existing", false, "Overwrite existing pending actions")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	return cmd
}

func newOrchestrateTaskEscalateThreadCmd(deps commandDeps) *cobra.Command {
	var workItemID int64
	var reason string
	var threadTitle string
	var actorProfile string
	var sourceSession string
	var inviteProfiles string
	var inviteHumans string
	var forceNew bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "escalate-thread",
		Short: "Escalate a task into a coordination thread",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := []string{"task", "escalate-thread"}
			if cmd.Flags().Changed("work-item-id") {
				forwardArgs = append(forwardArgs, "--work-item-id", strconv.FormatInt(workItemID, 10))
			}
			if cmd.Flags().Changed("reason") {
				forwardArgs = append(forwardArgs, "--reason", reason)
			}
			if cmd.Flags().Changed("thread-title") {
				forwardArgs = append(forwardArgs, "--thread-title", threadTitle)
			}
			if cmd.Flags().Changed("actor-profile") {
				forwardArgs = append(forwardArgs, "--actor-profile", actorProfile)
			}
			if cmd.Flags().Changed("source-session") {
				forwardArgs = append(forwardArgs, "--source-session", sourceSession)
			}
			if cmd.Flags().Changed("invite-profiles") && strings.TrimSpace(inviteProfiles) != "" {
				forwardArgs = append(forwardArgs, "--invite-profiles", inviteProfiles)
			}
			if cmd.Flags().Changed("invite-humans") && strings.TrimSpace(inviteHumans) != "" {
				forwardArgs = append(forwardArgs, "--invite-humans", inviteHumans)
			}
			if cmd.Flags().Changed("force-new") && forceNew {
				forwardArgs = append(forwardArgs, "--force-new")
			}
			if cmd.Flags().Changed("json") && jsonOutput {
				forwardArgs = append(forwardArgs, "--json")
			}
			return deps.runOrchestrate(forwardArgs)
		},
	}
	cmd.Flags().Int64Var(&workItemID, "work-item-id", 0, "Work item ID")
	cmd.Flags().StringVar(&reason, "reason", "", "Escalation reason")
	cmd.Flags().StringVar(&threadTitle, "thread-title", "", "Thread title")
	cmd.Flags().StringVar(&actorProfile, "actor-profile", "", "Actor profile ID")
	cmd.Flags().StringVar(&sourceSession, "source-session", "", "Source chat session ID")
	cmd.Flags().StringVar(&inviteProfiles, "invite-profiles", "", "Comma-separated invited profile IDs")
	cmd.Flags().StringVar(&inviteHumans, "invite-humans", "", "Comma-separated invited human IDs")
	cmd.Flags().BoolVar(&forceNew, "force-new", false, "Force creation of a new thread")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON output")
	return cmd
}

func appendReassignFlags(cmd *cobra.Command, forwardArgs *[]string, workItemID int64, profile string, reason string, actorProfile string, sourceSession string, jsonOutput bool) {
	if cmd.Flags().Changed("work-item-id") {
		*forwardArgs = append(*forwardArgs, "--work-item-id", strconv.FormatInt(workItemID, 10))
	}
	if cmd.Flags().Changed("profile") {
		*forwardArgs = append(*forwardArgs, "--profile", profile)
	}
	if cmd.Flags().Changed("reason") {
		*forwardArgs = append(*forwardArgs, "--reason", reason)
	}
	if cmd.Flags().Changed("actor-profile") {
		*forwardArgs = append(*forwardArgs, "--actor-profile", actorProfile)
	}
	if cmd.Flags().Changed("source-session") {
		*forwardArgs = append(*forwardArgs, "--source-session", sourceSession)
	}
	if cmd.Flags().Changed("json") && jsonOutput {
		*forwardArgs = append(*forwardArgs, "--json")
	}
}
