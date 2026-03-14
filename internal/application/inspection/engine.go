package inspection

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Store combines the persistence interfaces needed by the inspection engine.
type Store interface {
	core.InspectionStore
	core.AnalyticsStore
	core.WorkItemStore
	core.ActionStore
	core.RunStore
	core.UsageStore
	core.ActionSignalStore
}

// EventPublisher publishes domain events.
type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

// Engine performs inspection runs: collects system health data, identifies
// findings, detects recurrence patterns, and produces evolution insights.
type Engine struct {
	store Store
	bus   EventPublisher
}

// New creates a new inspection engine.
func New(store Store, bus EventPublisher) *Engine {
	return &Engine{store: store, bus: bus}
}

// RunInspection executes a full inspection cycle for the given time window.
func (e *Engine) RunInspection(ctx context.Context, trigger core.InspectionTrigger, projectID *int64, periodStart, periodEnd time.Time) (*core.InspectionReport, error) {
	report := &core.InspectionReport{
		ProjectID:   projectID,
		Status:      core.InspectionStatusRunning,
		Trigger:     trigger,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		CreatedAt:   time.Now(),
	}

	id, err := e.store.CreateInspection(ctx, report)
	if err != nil {
		return nil, fmt.Errorf("create inspection: %w", err)
	}
	report.ID = id

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventInspectionStarted,
		Data:      map[string]any{"inspection_id": id},
		Timestamp: time.Now(),
	})

	slog.Info("inspection started", "id", id, "trigger", trigger, "period_start", periodStart, "period_end", periodEnd)

	// Phase 1: Collect snapshot.
	snapshot, err := e.collectSnapshot(ctx, projectID, periodStart, periodEnd)
	if err != nil {
		return e.failInspection(ctx, report, fmt.Errorf("collect snapshot: %w", err))
	}
	report.Snapshot = snapshot

	// Phase 2: Detect findings.
	findings, err := e.detectFindings(ctx, report, snapshot, periodStart, periodEnd)
	if err != nil {
		return e.failInspection(ctx, report, fmt.Errorf("detect findings: %w", err))
	}
	report.Findings = findings

	// Phase 3: Generate evolution insights.
	insights := e.generateInsights(ctx, report, findings)
	report.Insights = insights

	// Phase 4: Suggest skills from recurring patterns.
	report.SuggestedSkills = e.suggestSkills(findings)

	// Phase 5: Generate summary.
	report.Summary = e.generateSummary(report)

	// Mark completed.
	now := time.Now()
	report.Status = core.InspectionStatusCompleted
	report.FinishedAt = &now
	if err := e.store.UpdateInspection(ctx, report); err != nil {
		slog.Error("failed to update inspection", "id", id, "error", err)
	}

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventInspectionCompleted,
		Data:      map[string]any{"inspection_id": id, "finding_count": len(findings), "insight_count": len(insights)},
		Timestamp: time.Now(),
	})

	slog.Info("inspection completed", "id", id, "findings", len(findings), "insights", len(insights))
	return report, nil
}

func (e *Engine) failInspection(ctx context.Context, report *core.InspectionReport, err error) (*core.InspectionReport, error) {
	now := time.Now()
	report.Status = core.InspectionStatusFailed
	report.ErrorMessage = err.Error()
	report.FinishedAt = &now
	_ = e.store.UpdateInspection(ctx, report)

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventInspectionFailed,
		Data:      map[string]any{"inspection_id": report.ID, "error": err.Error()},
		Timestamp: time.Now(),
	})
	return report, err
}

func (e *Engine) collectSnapshot(ctx context.Context, projectID *int64, since, until time.Time) (*core.InspectionSnapshot, error) {
	filter := core.AnalyticsFilter{
		ProjectID: projectID,
		Since:     &since,
		Until:     &until,
		Limit:     10,
	}

	snapshot := &core.InspectionSnapshot{}

	// Status distribution.
	statusDist, err := e.store.WorkItemStatusDistribution(ctx, filter)
	if err != nil {
		return nil, err
	}
	snapshot.StatusDistribution = statusDist
	for _, s := range statusDist {
		snapshot.TotalWorkItems += s.Count
		switch core.WorkItemStatus(s.Status) {
		case core.WorkItemRunning, core.WorkItemQueued:
			snapshot.ActiveWorkItems += s.Count
		case core.WorkItemFailed:
			snapshot.FailedWorkItems += s.Count
		case core.WorkItemBlocked:
			snapshot.BlockedWorkItems += s.Count
		}
	}

	if snapshot.TotalWorkItems > 0 {
		doneCount := 0
		for _, s := range statusDist {
			if core.WorkItemStatus(s.Status) == core.WorkItemDone {
				doneCount = s.Count
			}
		}
		snapshot.SuccessRate = float64(doneCount) / float64(snapshot.TotalWorkItems)
	}

	// Error breakdown.
	errors, err := e.store.ErrorBreakdown(ctx, filter)
	if err != nil {
		slog.Warn("inspection: error breakdown failed", "error", err)
	} else {
		snapshot.TopErrors = errors
		for _, e := range errors {
			snapshot.FailedRuns += e.Count
		}
	}

	// Bottlenecks.
	bottlenecks, err := e.store.WorkItemBottleneckActions(ctx, filter)
	if err != nil {
		slog.Warn("inspection: bottleneck query failed", "error", err)
	} else {
		snapshot.TopBottlenecks = bottlenecks
	}

	// Duration stats for average.
	durations, err := e.store.RunDurationStats(ctx, filter)
	if err != nil {
		slog.Warn("inspection: duration stats failed", "error", err)
	} else if len(durations) > 0 {
		total := 0.0
		count := 0
		for _, d := range durations {
			total += d.AvgDurationS * float64(d.RunCount)
			count += d.RunCount
		}
		if count > 0 {
			snapshot.AvgDurationS = total / float64(count)
			snapshot.TotalRuns = count
		}
	}

	// Usage totals.
	usageTotals, err := e.store.UsageTotals(ctx, filter)
	if err != nil {
		slog.Warn("inspection: usage totals failed", "error", err)
	} else if usageTotals != nil {
		snapshot.TotalTokens = usageTotals.TotalTokens
	}

	return snapshot, nil
}

func (e *Engine) detectFindings(ctx context.Context, report *core.InspectionReport, snapshot *core.InspectionSnapshot, since, until time.Time) ([]core.InspectionFinding, error) {
	var findings []core.InspectionFinding
	filter := core.AnalyticsFilter{
		ProjectID: report.ProjectID,
		Since:     &since,
		Until:     &until,
		Limit:     20,
	}

	// Finding 1: Blocked work items.
	if snapshot.BlockedWorkItems > 0 {
		findings = append(findings, core.InspectionFinding{
			InspectionID: report.ID,
			Category:     core.CategoryBlocker,
			Severity:     core.SeverityHigh,
			Title:        fmt.Sprintf("%d work items are blocked", snapshot.BlockedWorkItems),
			Description:  "Blocked work items cannot make progress without human intervention or external dependency resolution.",
			Recommendation: "Review blocked items and resolve dependencies or provide manual unblock signals.",
		})
	}

	// Finding 2: High failure rate.
	if snapshot.TotalWorkItems > 0 && snapshot.FailedWorkItems > 0 {
		failRate := float64(snapshot.FailedWorkItems) / float64(snapshot.TotalWorkItems)
		if failRate > 0.3 {
			findings = append(findings, core.InspectionFinding{
				InspectionID: report.ID,
				Category:     core.CategoryFailure,
				Severity:     severityFromRate(failRate),
				Title:        fmt.Sprintf("High failure rate: %.0f%% of work items failed", failRate*100),
				Description:  fmt.Sprintf("%d out of %d work items failed in this period.", snapshot.FailedWorkItems, snapshot.TotalWorkItems),
				Evidence:     fmt.Sprintf("failure_rate=%.2f, failed=%d, total=%d", failRate, snapshot.FailedWorkItems, snapshot.TotalWorkItems),
				Recommendation: "Analyze error patterns below and address the root causes. Consider adding retry logic or error-specific skills.",
			})
		}
	}

	// Finding 3: Recent failures with details.
	recentFailures, err := e.store.RecentFailures(ctx, filter)
	if err != nil {
		slog.Warn("inspection: recent failures query failed", "error", err)
	} else {
		for _, f := range recentFailures {
			recurrence, _ := e.store.GetFindingRecurrenceCount(ctx, core.CategoryFailure, &f.WorkItemID, &f.ActionID)
			recurring := recurrence > 0

			finding := core.InspectionFinding{
				InspectionID:    report.ID,
				Category:        core.CategoryFailure,
				Severity:        core.SeverityMedium,
				Title:           fmt.Sprintf("Action '%s' failed in '%s'", f.ActionName, f.WorkItemTitle),
				Description:     f.ErrorMessage,
				Evidence:        fmt.Sprintf("error_kind=%s, attempt=%d, duration=%.1fs", f.ErrorKind, f.Attempt, f.DurationS),
				WorkItemID:      &f.WorkItemID,
				ActionID:        &f.ActionID,
				RunID:           &f.RunID,
				ProjectID:       f.ProjectID,
				Recommendation:  recommendationForErrorKind(f.ErrorKind),
				Recurring:       recurring,
				OccurrenceCount: recurrence + 1,
			}
			if recurring {
				finding.Severity = core.SeverityHigh
			}
			findings = append(findings, finding)
		}
	}

	// Finding 4: Bottleneck actions.
	for _, b := range snapshot.TopBottlenecks {
		if b.FailRate > 0.5 || b.AvgDurationS > 300 {
			sev := core.SeverityMedium
			if b.FailRate > 0.7 {
				sev = core.SeverityHigh
			}
			findings = append(findings, core.InspectionFinding{
				InspectionID:   report.ID,
				Category:       core.CategoryBottleneck,
				Severity:       sev,
				Title:          fmt.Sprintf("Bottleneck: '%s' (avg %.0fs, %.0f%% fail rate)", b.ActionName, b.AvgDurationS, b.FailRate*100),
				Description:    fmt.Sprintf("Action '%s' in work item '%s' runs slowly or fails frequently.", b.ActionName, b.WorkItemTitle),
				Evidence:       fmt.Sprintf("avg_duration=%.1fs, max_duration=%.1fs, runs=%d, fails=%d", b.AvgDurationS, b.MaxDurationS, b.RunCount, b.FailCount),
				WorkItemID:     &b.WorkItemID,
				ActionID:       &b.ActionID,
				ProjectID:      b.ProjectID,
				Recommendation: "Break down large actions into smaller steps. Add specific error handling or timeout configuration.",
			})
		}
	}

	// Finding 5: Error pattern concentration.
	for _, ek := range snapshot.TopErrors {
		if ek.Count >= 3 {
			findings = append(findings, core.InspectionFinding{
				InspectionID:   report.ID,
				Category:       core.CategoryPattern,
				Severity:       core.SeverityMedium,
				Title:          fmt.Sprintf("Recurring error pattern: '%s' (%d occurrences, %.0f%%)", ek.ErrorKind, ek.Count, ek.Pct*100),
				Description:    fmt.Sprintf("Error kind '%s' accounts for %.0f%% of all errors.", ek.ErrorKind, ek.Pct*100),
				Evidence:       fmt.Sprintf("count=%d, percentage=%.1f%%", ek.Count, ek.Pct*100),
				Recommendation: recommendationForErrorKind(ek.ErrorKind),
			})
		}
	}

	// Persist all findings.
	for i := range findings {
		findings[i].CreatedAt = time.Now()
		if _, err := e.store.CreateFinding(ctx, &findings[i]); err != nil {
			slog.Warn("inspection: failed to persist finding", "title", findings[i].Title, "error", err)
		}
	}

	return findings, nil
}

func (e *Engine) generateInsights(ctx context.Context, report *core.InspectionReport, findings []core.InspectionFinding) []core.InspectionInsight {
	var insights []core.InspectionInsight

	// Count recurring findings.
	recurringCount := 0
	for _, f := range findings {
		if f.Recurring {
			recurringCount++
		}
	}

	// Insight 1: Recurrence pattern.
	if recurringCount > 0 {
		insight := core.InspectionInsight{
			InspectionID: report.ID,
			Type:         "pattern",
			Title:        fmt.Sprintf("%d recurring issues detected", recurringCount),
			Description:  "These issues have appeared in previous inspections and are not yet resolved. Recurring problems indicate systematic gaps that need structural solutions.",
			Trend:        "degrading",
			ActionItems: []string{
				"Review recurring findings and create dedicated work items for root-cause fixes",
				"Consider creating skills that automate the resolution of these patterns",
				"Add acceptance criteria or gate checks to prevent these issues from recurring",
			},
			CreatedAt: time.Now(),
		}
		insights = append(insights, insight)
	}

	// Insight 2: Category distribution.
	catCounts := map[core.FindingCategory]int{}
	for _, f := range findings {
		catCounts[f.Category]++
	}

	if len(catCounts) > 0 {
		dominant := core.FindingCategory("")
		maxCount := 0
		for cat, count := range catCounts {
			if count > maxCount {
				dominant = cat
				maxCount = count
			}
		}

		insight := core.InspectionInsight{
			InspectionID: report.ID,
			Type:         "lesson",
			Title:        fmt.Sprintf("Dominant issue category: %s (%d findings)", dominant, maxCount),
			Description:  fmt.Sprintf("The most common finding category is '%s'. Focusing improvement efforts here will have the highest impact.", dominant),
			Trend:        "stable",
			ActionItems:  actionItemsForCategory(dominant),
			CreatedAt:    time.Now(),
		}
		insights = append(insights, insight)
	}

	// Insight 3: Overall health assessment.
	healthTrend := "stable"
	if len(findings) == 0 {
		healthTrend = "improving"
	} else if len(findings) > 5 || recurringCount > 2 {
		healthTrend = "degrading"
	}

	insight := core.InspectionInsight{
		InspectionID: report.ID,
		Type:         "optimization",
		Title:        fmt.Sprintf("System health: %s (%d total findings)", healthTrend, len(findings)),
		Description:  generateHealthDescription(healthTrend, len(findings), recurringCount),
		Trend:        healthTrend,
		ActionItems:  generateHealthActions(healthTrend, findings),
		CreatedAt:    time.Now(),
	}
	insights = append(insights, insight)

	// Persist insights.
	for i := range insights {
		if _, err := e.store.CreateInsight(ctx, &insights[i]); err != nil {
			slog.Warn("inspection: failed to persist insight", "title", insights[i].Title, "error", err)
		}
	}

	return insights
}

func (e *Engine) suggestSkills(findings []core.InspectionFinding) []core.SuggestedSkill {
	var skills []core.SuggestedSkill

	// Look for recurring failure patterns that could become skills.
	errorPatterns := map[string]int{}
	for _, f := range findings {
		if f.Recurring && f.Category == core.CategoryFailure {
			errorPatterns[f.Title]++
		}
	}

	for pattern, count := range errorPatterns {
		if count >= 2 {
			skills = append(skills, core.SuggestedSkill{
				Name:        "auto-fix-" + sanitizeSkillName(pattern),
				Description: fmt.Sprintf("Automated resolution for recurring failure: %s", pattern),
				Rationale:   fmt.Sprintf("This failure pattern has occurred %d times across inspections.", count),
			})
		}
	}

	// Suggest blocker-resolution skill if blockers are common.
	blockerCount := 0
	for _, f := range findings {
		if f.Category == core.CategoryBlocker {
			blockerCount++
		}
	}
	if blockerCount >= 2 {
		skills = append(skills, core.SuggestedSkill{
			Name:        "blocker-resolution",
			Description: "Automated detection and resolution of common blocker patterns",
			Rationale:   fmt.Sprintf("%d blocker findings detected. A dedicated skill could pre-emptively handle these.", blockerCount),
		})
	}

	// Suggest bottleneck optimization skill.
	bottleneckCount := 0
	for _, f := range findings {
		if f.Category == core.CategoryBottleneck {
			bottleneckCount++
		}
	}
	if bottleneckCount >= 2 {
		skills = append(skills, core.SuggestedSkill{
			Name:        "perf-optimization",
			Description: "Performance optimization patterns for slow-running actions",
			Rationale:   fmt.Sprintf("%d bottleneck findings detected. A skill could suggest parallelization or caching strategies.", bottleneckCount),
		})
	}

	return skills
}

func (e *Engine) generateSummary(report *core.InspectionReport) string {
	totalFindings := len(report.Findings)
	totalInsights := len(report.Insights)
	totalSkills := len(report.SuggestedSkills)

	critical := 0
	high := 0
	for _, f := range report.Findings {
		switch f.Severity {
		case core.SeverityCritical:
			critical++
		case core.SeverityHigh:
			high++
		}
	}

	summary := fmt.Sprintf("Inspection completed for period %s to %s. ",
		report.PeriodStart.Format("2006-01-02 15:04"),
		report.PeriodEnd.Format("2006-01-02 15:04"))

	if totalFindings == 0 {
		summary += "No issues found. System is healthy."
	} else {
		summary += fmt.Sprintf("Found %d issues", totalFindings)
		if critical > 0 || high > 0 {
			summary += fmt.Sprintf(" (%d critical, %d high severity)", critical, high)
		}
		summary += fmt.Sprintf(", generated %d evolution insights", totalInsights)
		if totalSkills > 0 {
			summary += fmt.Sprintf(", suggested %d new skills", totalSkills)
		}
		summary += "."
	}

	if report.Snapshot != nil {
		summary += fmt.Sprintf(" Snapshot: %d work items, %.0f%% success rate, %d total runs.",
			report.Snapshot.TotalWorkItems,
			report.Snapshot.SuccessRate*100,
			report.Snapshot.TotalRuns)
	}

	return summary
}

// -- Helpers --

func severityFromRate(rate float64) core.FindingSeverity {
	if rate > 0.7 {
		return core.SeverityCritical
	}
	if rate > 0.5 {
		return core.SeverityHigh
	}
	return core.SeverityMedium
}

func recommendationForErrorKind(kind core.ErrorKind) string {
	switch kind {
	case core.ErrKindTransient:
		return "Transient errors often resolve on retry. Increase max_retries or add exponential backoff."
	case core.ErrKindPermanent:
		return "Permanent errors need code or configuration fixes. Review the failing action's implementation."
	case core.ErrKindNeedHelp:
		return "Agent explicitly requested help. Review the action context and provide clearer instructions or additional skills."
	default:
		return "Investigate the error logs for this action and consider adding specific error handling."
	}
}

func actionItemsForCategory(cat core.FindingCategory) []string {
	switch cat {
	case core.CategoryBlocker:
		return []string{
			"Review all blocked work items and resolve external dependencies",
			"Set up automatic notifications for blocked items",
			"Consider adding timeout-based auto-escalation for blocked actions",
		}
	case core.CategoryFailure:
		return []string{
			"Analyze error patterns and add targeted error handling",
			"Review and improve action acceptance criteria",
			"Consider creating error-recovery skills for common failure modes",
		}
	case core.CategoryBottleneck:
		return []string{
			"Profile slow actions and identify optimization opportunities",
			"Consider breaking large actions into smaller parallel steps",
			"Review action timeouts and adjust accordingly",
		}
	case core.CategoryPattern:
		return []string{
			"Create skills that address recurring error patterns",
			"Add pre-flight checks to prevent known failure modes",
			"Document patterns in CLAUDE.md for better agent guidance",
		}
	default:
		return []string{"Review findings and create targeted improvement work items"}
	}
}

func generateHealthDescription(trend string, findings, recurring int) string {
	switch trend {
	case "improving":
		return "No issues were detected in this inspection period. The system is operating smoothly."
	case "degrading":
		return fmt.Sprintf("System health is declining with %d findings (%d recurring). Immediate attention is needed on recurring issues.", findings, recurring)
	default:
		return fmt.Sprintf("System health is stable with %d findings. Monitor recurring issues and address high-severity items.", findings)
	}
}

func generateHealthActions(trend string, findings []core.InspectionFinding) []string {
	switch trend {
	case "improving":
		return []string{
			"Continue monitoring with regular inspections",
			"Consider reducing inspection frequency if trend continues",
		}
	case "degrading":
		return []string{
			"Address all critical and high-severity findings immediately",
			"Create dedicated work items for each recurring issue",
			"Consider increasing inspection frequency to daily",
			"Review recent changes that may have introduced regressions",
		}
	default:
		return []string{
			"Address high-severity findings when possible",
			"Monitor trends across inspections for early warning signs",
		}
	}
}

func sanitizeSkillName(s string) string {
	// Simple sanitization: take first few words, lowercase, hyphenate.
	result := ""
	wordCount := 0
	for _, c := range s {
		if wordCount >= 3 {
			break
		}
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			result += string(c)
		} else if c >= 'A' && c <= 'Z' {
			result += string(c + 32)
		} else if c == ' ' || c == '_' || c == '-' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result += "-"
				wordCount++
			}
		}
	}
	if len(result) > 30 {
		result = result[:30]
	}
	// Trim trailing hyphens.
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	if result == "" {
		result = "pattern"
	}
	return result
}
