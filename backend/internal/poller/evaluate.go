// Package poller orchestrates periodic media library polling and capacity evaluation.
package poller

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/engine"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
	"capacitarr/internal/services"
)

// evaluateAndCleanDisk scores all media items on a disk group and, when the
// disk is above threshold, queues candidates for deletion or approval.
// Returns the number of items queued for deletion. The acc accumulator
// collects per-run metrics across multiple disk group evaluations.
func (p *Poller) evaluateAndCleanDisk(acc *RunAccumulator, group db.DiskGroup, allItems []integrations.MediaItem, registry *integrations.IntegrationRegistry, runStatsID uint, prefs db.PreferenceSet, weights map[string]int, rules []db.CustomRule, evalCtx *engine.EvaluationContext) int {
	effectiveTotal := group.EffectiveTotalBytes()
	if effectiveTotal == 0 {
		slog.Warn("Disk group effective total is 0, skipping evaluation",
			"component", "poller", "mount", group.MountPath,
			"totalBytes", group.TotalBytes, "override", group.TotalBytesOverride)
		return 0
	}
	currentPct := float64(group.UsedBytes) / float64(effectiveTotal) * 100

	// Get or create a per-group accumulator and record disk usage info.
	groupAcc := acc.GetOrCreate(group.ID, group.MountPath, group.Mode)
	groupAcc.DiskUsagePct = currentPct
	groupAcc.DiskThreshold = group.ThresholdPct
	groupAcc.DiskTargetPct = group.TargetPct

	// ── Sunset mode: special threshold handling ─────────────────────────
	// Sunset mode uses sunsetPct (lower) as the primary trigger and thresholdPct
	// (higher) as the escalation safety net. The mode switch determines which
	// threshold matters.
	if group.Mode == db.ModeSunset {
		return p.evaluateSunsetMode(acc, group, allItems, registry, runStatsID, prefs, weights, rules, evalCtx, effectiveTotal, currentPct)
	}

	// ── Standard modes: dry-run, approval, auto ─────────────────────────
	if currentPct < group.ThresholdPct {
		slog.Debug("Disk within threshold, no action needed", "component", "poller",
			"mount", group.MountPath, "usedPct", fmt.Sprintf("%.1f", currentPct),
			"threshold", group.ThresholdPct, "mode", group.Mode)

		// Clear stale approval queue items for this specific disk group.
		// Without this, items queued when the group was above threshold would
		// linger until ALL groups drop below threshold (the global ClearQueue).
		if cleared, err := p.reg.Approval.ClearQueueForDiskGroup(group.ID); err != nil {
			slog.Error("Failed to clear approval queue for disk group",
				"component", "poller", "diskGroupID", group.ID, "error", err)
		} else if cleared > 0 {
			slog.Info("Approval queue cleared for disk group (below threshold)",
				"component", "poller", "mount", group.MountPath, "cleared", cleared)
		}

		return 0
	}

	slog.Info("Disk threshold breached, evaluating media for deletion", "component", "poller",
		"mount", group.MountPath, "currentPct", fmt.Sprintf("%.1f", currentPct), "threshold", group.ThresholdPct)

	p.reg.Bus.Publish(events.ThresholdBreachedEvent{
		MountPath:    group.MountPath,
		CurrentPct:   currentPct,
		ThresholdPct: group.ThresholdPct,
		TargetPct:    group.TargetPct,
	})

	// Filter items on this mount — normalize paths for cross-platform
	// compatibility (Windows *arr instances return backslash paths).
	normalizedMount := normalizePath(group.MountPath)
	var diskItems []integrations.MediaItem
	for _, item := range allItems {
		if strings.HasPrefix(normalizePath(item.Path), normalizedMount) {
			diskItems = append(diskItems, item)
		}
	}

	if len(diskItems) == 0 {
		slog.Warn("No items matched disk mount path — approval queue cannot be populated",
			"component", "poller", "mount", group.MountPath,
			"normalizedMount", normalizedMount, "totalItems", len(allItems))
		if len(allItems) > 0 {
			sampleCount := 3
			if len(allItems) < sampleCount {
				sampleCount = len(allItems)
			}
			for i := 0; i < sampleCount; i++ {
				slog.Debug("Sample item path for mount mismatch diagnosis",
					"component", "poller", "itemPath", normalizePath(allItems[i].Path),
					"mount", normalizedMount)
			}
		}
	}
	slog.Debug("Items on disk mount", "component", "poller",
		"mount", group.MountPath, "itemCount", len(diskItems))

	// Use the extracted Evaluator for scoring + categorization
	evaluator := engine.NewEvaluator()
	evalResult := evaluator.Evaluate(diskItems, weights, rules, prefs.TiebreakerMethod, evalCtx)
	groupAcc.Evaluated += int64(evalResult.TotalCount)
	groupAcc.Protected += int64(len(evalResult.Protected))

	slog.Debug("Evaluation summary", "component", "poller",
		"mount", group.MountPath,
		"evaluated", evalResult.TotalCount,
		"protected", len(evalResult.Protected),
		"candidates", len(evalResult.Candidates))

	targetBytesToFree := int64((currentPct - group.TargetPct) / 100.0 * float64(effectiveTotal))
	if targetBytesToFree <= 0 {
		slog.Warn("Target bytes to free is zero or negative, skipping evaluation",
			"component", "poller", "mount", group.MountPath,
			"currentPct", fmt.Sprintf("%.1f", currentPct),
			"targetPct", group.TargetPct,
			"targetBytesToFree", targetBytesToFree)
		return 0
	}

	// Get deletion candidates from the evaluator result
	candidates := evalResult.CandidatesForDeletion(targetBytesToFree)

	slog.Info("Candidate selection for approval/deletion", "component", "poller",
		"mount", group.MountPath,
		"executionMode", prefs.DefaultDiskGroupMode,
		"totalCandidates", len(evalResult.Candidates),
		"selectedCandidates", len(candidates),
		"targetBytesToFree", targetBytesToFree)

	// Pre-build set of shows that have season-level entries in the candidates.
	// When season entries exist, prefer them over show-level entries so each season
	// can be individually approved/snoozed/deleted in the approval queue.
	showsWithSeasons := make(map[string]bool)
	for _, ev := range candidates {
		if ev.Item.Type == integrations.MediaTypeSeason && ev.Item.ShowTitle != "" {
			showsWithSeasons[ev.Item.ShowTitle] = true
		}
	}

	// Track which items are still needed this cycle (for queue reconciliation).
	// Keys are "MediaName|MediaType" strings matching the approval queue schema.
	neededKeys := make(map[string]bool)

	// Pre-fetch all snoozed keys for this disk group in a single query.
	// This replaces per-item IsSnoozed() calls (N queries → 1).
	snoozedKeys, snoozedErr := p.reg.Approval.ListSnoozedKeys(group.ID)
	if snoozedErr != nil {
		slog.Error("Failed to pre-fetch snoozed keys, falling back to empty set",
			"component", "poller", "mount", group.MountPath, "error", snoozedErr)
		snoozedKeys = make(map[string]bool)
	}

	var bytesFreed int64
	var deletionsQueued int
	var skippedZeroScore int
	var skippedDedup int
	var skippedSnoozed int
	var skippedCollectionProtected int

	// Collect approval-mode items for batch upsert (Phase 2 optimization).
	var pendingBatch []db.ApprovalQueueItem

	// Track collections already expanded to avoid duplicate processing when
	// multiple items from the same collection appear in the candidate list.
	expandedCollections := make(map[string]bool)

	// Pre-fetch integration configs for collection deletion checks.
	// Uses a cache to avoid repeated DB lookups for the same integration.
	integrationConfigCache := make(map[uint]*db.IntegrationConfig)
	getIntegrationConfig := func(id uint) *db.IntegrationConfig {
		if cfg, ok := integrationConfigCache[id]; ok {
			return cfg
		}
		cfg, err := p.reg.Integration.GetByID(id)
		if err != nil {
			return nil
		}
		integrationConfigCache[id] = cfg
		return cfg
	}

	for _, ev := range candidates {
		if bytesFreed >= targetBytesToFree {
			break
		}
		if ev.IsProtected || ev.Score <= 0 {
			skippedZeroScore++
			continue
		}

		// Dedup: skip show-level entries when season entries exist for the same show.
		// Season entries allow granular per-season approval and deletion.
		if ev.Item.Type == integrations.MediaTypeShow {
			if showsWithSeasons[ev.Item.Title] {
				skippedDedup++
				continue
			}
		}

		// Skip items that are currently snoozed (rejected with an active snooze window).
		// This check runs in ALL execution modes so items snoozed from the deletion queue
		// in auto/dry-run mode are also respected by the engine.
		// Uses pre-fetched snoozedKeys map for O(1) lookup instead of per-item DB query.
		snoozedKey := db.MediaKey(ev.Item.Title, string(ev.Item.Type))
		if snoozedKeys[snoozedKey] {
			skippedSnoozed++
			slog.Debug("Skipping snoozed item", "component", "poller", "media", ev.Item.Title)
			continue
		}

		slog.Debug("Deletion candidate", "component", "poller",
			"media", ev.Item.Title, "score", fmt.Sprintf("%.4f", ev.Score),
			"size", ev.Item.SizeBytes, "reason", ev.Reason)

		// ── Collection expansion ─────────────────────────────────────────
		// When collection deletion is enabled on ANY integration that sourced
		// collection data for this item, expand the single candidate into all
		// collection members. Multiple collections may trigger — the union of
		// all resolved members is used.
		type processItem struct {
			item            integrations.MediaItem
			score           float64
			factors         []engine.ScoreFactor
			collectionGroup string // non-empty if part of a collection
		}
		itemsToProcess := []processItem{{item: ev.Item, score: ev.Score, factors: ev.Factors}}

		// Find all collections where the source integration has collectionDeletion ON.
		var enabledCollections []string
		for _, colName := range ev.Item.Collections {
			sourceID, ok := ev.Item.CollectionSources[colName]
			if !ok {
				sourceID = ev.Item.IntegrationID // fallback: assume item's own integration
			}
			sourceCfg := getIntegrationConfig(sourceID)
			if sourceCfg != nil && sourceCfg.CollectionDeletion {
				enabledCollections = append(enabledCollections, colName)
			}
		}

		if len(enabledCollections) > 0 {
			// Check if ALL enabled collections were already expanded
			allExpanded := true
			for _, colName := range enabledCollections {
				if !expandedCollections[colName] {
					allExpanded = false
					break
				}
			}
			if allExpanded {
				slog.Debug("Skipping already-expanded collection member", "component", "poller",
					"media", ev.Item.Title, "collections", strings.Join(enabledCollections, ", "))
				continue
			}

			// Resolve members for each enabled collection, merging results.
			// Uses two strategies: (1) CollectionResolver for *arr items (TMDb-based),
			// (2) allItems scan for media-server collections.
			memberDedup := make(map[string]bool) // ExternalID+IntegrationID → seen
			var allMembers []integrations.MediaItem
			var resolvedCollections []string

			for _, colName := range enabledCollections {
				if expandedCollections[colName] {
					continue
				}

				var members []integrations.MediaItem

				// Strategy 1: Use CollectionResolver if the item's integration has one
				if resolver, ok := registry.CollectionResolver(ev.Item.IntegrationID); ok {
					resolved, resolveErr := resolver.ResolveCollectionMembers(ev.Item)
					if resolveErr != nil {
						slog.Error("Collection resolution failed, falling back to allItems scan", "component", "poller",
							"media", ev.Item.Title, "collection", colName, "error", resolveErr)
					} else if len(resolved) > 1 {
						members = resolved
					}
				}

				// Strategy 2: Scan allItems for siblings with the same collection name.
				// This handles media-server collections that have no dedicated resolver.
				if len(members) == 0 {
					for _, ai := range allItems {
						for _, c := range ai.Collections {
							if c == colName {
								members = append(members, ai)
								break
							}
						}
					}
				}

				if len(members) <= 1 {
					expandedCollections[colName] = true
					continue // No siblings — nothing to expand
				}

				// Deduplicate into allMembers
				for _, member := range members {
					key := member.ExternalID + "|" + fmt.Sprintf("%d", member.IntegrationID)
					if !memberDedup[key] {
						memberDedup[key] = true
						allMembers = append(allMembers, member)
					}
				}
				resolvedCollections = append(resolvedCollections, colName)
				expandedCollections[colName] = true
			}

			if len(allMembers) > 1 {
				collectionGroupName := strings.Join(resolvedCollections, ", ")

				// Check if any collection member is protected by always_keep rules
				memberProtected := false
				for _, member := range allMembers {
					isProtected, _, _, _ := engine.ApplyRulesExported(member, rules)
					if isProtected {
						slog.Info("Collection skipped — member has always_keep rule", "component", "poller",
							"trigger", ev.Item.Title, "collection", collectionGroupName, "protectedMember", member.Title)
						memberProtected = true
						break
					}
				}
				if memberProtected {
					skippedCollectionProtected++
					continue
				}

				// Check if any collection member is snoozed
				memberSnoozed := false
				for _, member := range allMembers {
					memberSnoozedKey := db.MediaKey(member.Title, string(member.Type))
					if snoozedKeys[memberSnoozedKey] {
						slog.Info("Collection skipped — member is snoozed", "component", "poller",
							"trigger", ev.Item.Title, "collection", collectionGroupName, "snoozedMember", member.Title)
						memberSnoozed = true
						break
					}
				}
				if memberSnoozed {
					skippedSnoozed++
					continue
				}

				// Expand: replace the single trigger item with all collection members
				itemsToProcess = make([]processItem, 0, len(allMembers))
				for _, member := range allMembers {
					itemsToProcess = append(itemsToProcess, processItem{
						item:            member,
						score:           ev.Score, // Use the trigger item's score for all members
						factors:         ev.Factors,
						collectionGroup: collectionGroupName,
					})
				}

				groupAcc.Collections++
				slog.Info("Collection expanded for deletion", "component", "poller",
					"trigger", ev.Item.Title, "collections", collectionGroupName, "memberCount", len(allMembers))
			}
		}

		// ── Process all items (single item or expanded collection) ────────
		for _, pi := range itemsToProcess {
			switch group.Mode {
			case db.ModeAuto:
				deleter, err := registry.Deleter(pi.item.IntegrationID)
				if err != nil {
					slog.Error("Integration not registered as MediaDeleter", "component", "poller",
						"integrationId", pi.item.IntegrationID, "error", err)
					continue
				}

				// Queue for background deletion via DeletionService
				diskGroupID := group.ID
				if err := p.reg.Deletion.QueueDeletion(services.DeleteJob{
					Client:          deleter,
					Item:            pi.item,
					Score:           pi.score,
					Factors:         pi.factors,
					Trigger:         db.TriggerEngine,
					RunStatsID:      runStatsID,
					DiskGroupID:     &diskGroupID,
					CollectionGroup: pi.collectionGroup,
					EnqueuedMode:    db.ModeAuto,
				}); err != nil {
					slog.Warn("Deletion queue full, skipping item", "component", "poller", "item", pi.item.Title)
					continue
				}
				groupAcc.Candidates++
				groupAcc.FreedBytes += pi.item.SizeBytes
				bytesFreed += pi.item.SizeBytes
				deletionsQueued++
			case db.ModeApproval:
				// Collect for batch upsert after the loop.
				factorsJSON, marshalErr := json.Marshal(pi.factors)
				if marshalErr != nil {
					slog.Error("Failed to marshal score factors", "component", "poller", "error", marshalErr)
					factorsJSON = []byte("[]")
				}
				diskGroupID := group.ID
				pendingBatch = append(pendingBatch, db.ApprovalQueueItem{
					MediaName:       pi.item.Title,
					MediaType:       string(pi.item.Type),
					ScoreDetails:    string(factorsJSON),
					SizeBytes:       pi.item.SizeBytes,
					Score:           pi.score,
					PosterURL:       pi.item.PosterURL,
					IntegrationID:   pi.item.IntegrationID,
					ExternalID:      pi.item.ExternalID,
					DiskGroupID:     &diskGroupID,
					Trigger:         db.TriggerEngine,
					CollectionGroup: pi.collectionGroup,
				})

				// Track this item as still-needed for post-loop reconciliation
				neededKeys[db.MediaKey(pi.item.Title, string(pi.item.Type))] = true

				bytesFreed += pi.item.SizeBytes
				groupAcc.Candidates++
				groupAcc.FreedBytes += pi.item.SizeBytes
				slog.Info("Engine action taken", "component", "poller",
					"media", pi.item.Title, "action", "queued_for_approval", "score", pi.score, "freed", pi.item.SizeBytes,
					"collectionGroup", pi.collectionGroup)
			default:
				// Dry-run mode: queue through DeletionService with ForceDryRun + UpsertAudit
				diskGroupID := group.ID
				if err := p.reg.Deletion.QueueDeletion(services.DeleteJob{
					Client:          nil, // Dry-run never calls DeleteMediaItem; nil-safe in processJob()
					Item:            pi.item,
					Score:           pi.score,
					Factors:         pi.factors,
					Trigger:         db.TriggerEngine,
					RunStatsID:      runStatsID,
					DiskGroupID:     &diskGroupID,
					ForceDryRun:     true,
					UpsertAudit:     true,
					CollectionGroup: pi.collectionGroup,
					EnqueuedMode:    db.ModeDryRun,
				}); err != nil {
					slog.Warn("Deletion queue full, skipping dry-run item", "component", "poller", "item", pi.item.Title)
					continue
				}
				bytesFreed += pi.item.SizeBytes
				deletionsQueued++
				groupAcc.Candidates++
				groupAcc.FreedBytes += pi.item.SizeBytes
				slog.Info("Engine action taken", "component", "poller",
					"media", pi.item.Title, "action", db.ActionDryDelete, "score", pi.score, "freed", pi.item.SizeBytes,
					"collectionGroup", pi.collectionGroup)
			}
		}
	}

	// Flush collected approval-mode items in a single batch transaction.
	// This replaces N individual UpsertPending() calls with one BulkUpsertPending().
	if len(pendingBatch) > 0 {
		created, updated, batchErr := p.reg.Approval.BulkUpsertPending(pendingBatch)
		if batchErr != nil {
			slog.Error("Failed to batch upsert approval queue items", "component", "poller",
				"mount", group.MountPath, "batchSize", len(pendingBatch), "error", batchErr)
		} else {
			slog.Info("Batch upserted approval queue items", "component", "poller",
				"mount", group.MountPath, "created", created, "updated", updated)
		}
	}

	// Per-cycle queue reconciliation: in approval mode, dismiss any pending items
	// for this disk group that are no longer in the "still-needed" set. This trims
	// stale entries that were added in previous cycles but are no longer candidates
	// (e.g., threshold was raised, scores changed, media was removed).
	if group.Mode == db.ModeApproval {
		if dismissed, reconcileErr := p.reg.Approval.ReconcileQueue(group.ID, neededKeys); reconcileErr != nil {
			slog.Error("Failed to reconcile approval queue", "component", "poller",
				"mount", group.MountPath, "error", reconcileErr)
		} else if dismissed > 0 {
			slog.Info("Approval queue reconciled", "component", "poller",
				"mount", group.MountPath, "dismissed", dismissed)
		}
	}

	// Diagnostic summary: log when candidates were found but all were skipped
	if len(candidates) > 0 && deletionsQueued == 0 && groupAcc.Candidates == 0 {
		slog.Warn("All candidates were skipped — nothing queued for approval/deletion",
			"component", "poller", "mount", group.MountPath,
			"mode", group.Mode,
			"candidates", len(candidates),
			"skippedZeroScore", skippedZeroScore,
			"skippedDedup", skippedDedup,
			"skippedSnoozed", skippedSnoozed,
			"skippedCollectionProtected", skippedCollectionProtected,
			"bytesFreedSoFar", bytesFreed)
	}

	return deletionsQueued
}

// evaluateSunsetMode handles the sunset-mode disk group evaluation.
// Both steps run independently each cycle:
//  1. Queue: if currentPct >= sunsetPct, score items and add to sunset_queue
//  2. Escalate: if currentPct >= criticalPct, force-expire from the queue to free space
//
// This ensures items are always marked for sunset once past the sunset threshold,
// even when the disk is simultaneously past critical. Escalation then processes
// items that were just queued (or already existed) from the sunset queue.
func (p *Poller) evaluateSunsetMode(acc *RunAccumulator, group db.DiskGroup, allItems []integrations.MediaItem, registry *integrations.IntegrationRegistry, _ uint, prefs db.PreferenceSet, weights map[string]int, rules []db.CustomRule, evalCtx *engine.EvaluationContext, effectiveTotal int64, currentPct float64) int {
	// Get the per-group accumulator (already created by evaluateAndCleanDisk
	// before dispatching to sunset mode).
	groupAcc := acc.GetOrCreate(group.ID, group.MountPath, group.Mode)

	// Validation: sunsetPct must be configured
	if group.SunsetPct == nil {
		slog.Warn("Sunset mode skipped — sunset threshold not configured",
			"component", "poller", "mount", group.MountPath, "diskGroupID", group.ID)
		p.reg.Bus.Publish(events.SunsetMisconfiguredEvent{
			DiskGroupID: group.ID,
			MountPath:   group.MountPath,
		})
		return 0
	}

	sunsetPct := *group.SunsetPct

	sunsetDeps := services.SunsetDeps{
		Registry:      registry,
		Deletion:      p.reg.Deletion,
		Engine:        p.reg.Engine,
		Settings:      p.reg.Settings,
		Preview:       p.reg.Preview,
		PosterOverlay: p.reg.PosterOverlay,
		Mapping:       p.reg.Mapping,
	}

	// Step 1: Queue items to sunset if sunsetPct is breached
	if currentPct >= sunsetPct {
		slog.Info("Sunset threshold breached, evaluating media for sunset queue", "component", "poller",
			"mount", group.MountPath, "currentPct", fmt.Sprintf("%.1f", currentPct),
			"sunsetPct", sunsetPct)

		// Filter items on this mount
		normalizedMount := normalizePath(group.MountPath)
		var diskItems []integrations.MediaItem
		for _, item := range allItems {
			if strings.HasPrefix(normalizePath(item.Path), normalizedMount) {
				diskItems = append(diskItems, item)
			}
		}

		// Score items
		evaluator := engine.NewEvaluator()
		evalResult := evaluator.Evaluate(diskItems, weights, rules, prefs.TiebreakerMethod, evalCtx)
		groupAcc.Evaluated += int64(evalResult.TotalCount)
		groupAcc.Protected += int64(len(evalResult.Protected))

		// Calculate how much to sunset (based on currentPct → targetPct range)
		targetBytesToFree := int64((currentPct - group.TargetPct) / 100.0 * float64(effectiveTotal))
		if targetBytesToFree > 0 {
			candidates := evalResult.CandidatesForDeletion(targetBytesToFree)

			// Pre-build set of already-sunsetted items for dedup
			sunsettedKeys, keysErr := p.reg.Sunset.ListSunsettedKeys(group.ID)
			if keysErr != nil {
				slog.Error("Failed to load sunsetted keys", "component", "poller", "error", keysErr)
				sunsettedKeys = make(map[string]bool)
			}

			// Calculate sunset deletion date
			deletionDate := time.Now().UTC().AddDate(0, 0, prefs.SunsetDays)

			var sunsetItems []db.SunsetQueueItem
			for _, candidate := range candidates {
				key := candidate.Item.Title + "|" + string(candidate.Item.Type)
				if sunsettedKeys[key] {
					continue // Already in sunset queue
				}

				factorsJSON, marshalErr := json.Marshal(candidate.Factors)
				if marshalErr != nil {
					slog.Error("Failed to marshal sunset candidate factors", "component", "poller",
						"mediaName", candidate.Item.Title, "error", marshalErr)
					continue
				}
				sunsetItems = append(sunsetItems, db.SunsetQueueItem{
					MediaName:       candidate.Item.Title,
					MediaType:       string(candidate.Item.Type),
					TmdbID:          &candidate.Item.TMDbID,
					IntegrationID:   candidate.Item.IntegrationID,
					ExternalID:      candidate.Item.ExternalID,
					SizeBytes:       candidate.Item.SizeBytes,
					Score:           candidate.Score,
					ScoreDetails:    string(factorsJSON),
					PosterURL:       candidate.Item.PosterURL,
					DiskGroupID:     group.ID,
					CollectionGroup: "", // Collection groups are handled by approval/auto mode expansion; sunset evaluates items individually
					Trigger:         db.TriggerEngine,
					DeletionDate:    deletionDate,
				})
			}

			if len(sunsetItems) > 0 {
				created, err := p.reg.Sunset.BulkQueueSunset(sunsetItems, sunsetDeps)
				if err != nil {
					slog.Error("Failed to queue sunset items", "component", "poller",
						"mount", group.MountPath, "error", err)
				} else {
					slog.Info("Sunset items queued", "component", "poller",
						"mount", group.MountPath, "count", created,
						"deletionDate", deletionDate.Format("2006-01-02"))
					groupAcc.Candidates += int64(created)
					groupAcc.SunsetQueued += created
				}
			}
		}
	} else {
		slog.Debug("Disk within sunset threshold, no action needed", "component", "poller",
			"mount", group.MountPath, "usedPct", fmt.Sprintf("%.1f", currentPct),
			"sunsetPct", sunsetPct)
	}

	// Step 2: Escalate if criticalPct is also breached (independent of step 1)
	if currentPct >= group.ThresholdPct {
		slog.Warn("Sunset escalation triggered — disk exceeds critical",
			"component", "poller", "mount", group.MountPath,
			"currentPct", fmt.Sprintf("%.1f", currentPct),
			"criticalPct", group.ThresholdPct,
			"targetPct", group.TargetPct)

		// Calculate bytes to free down to targetPct (NOT sunsetPct — preserves queue)
		targetBytes := int64((currentPct - group.TargetPct) / 100.0 * float64(effectiveTotal))
		freed, err := p.reg.Sunset.Escalate(group.ID, targetBytes, sunsetDeps)
		if err != nil {
			slog.Error("Sunset escalation failed", "component", "poller",
				"mount", group.MountPath, "error", err)
		}
		groupAcc.FreedBytes += freed
	}

	return 0 // Sunset mode doesn't queue immediate deletions
}
