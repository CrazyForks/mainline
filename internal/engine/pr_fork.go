package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mainline-org/mainline/internal/domain"
)

const forkPRCommentProvenanceKind = "fork_actor_log_preview"

func (s *Service) readForkPRCommentIntents(forkURL string, prNumber int) ([]domain.IntentView, error) {
	cfg, err := s.getTeamConfig()
	if err != nil {
		return nil, err
	}

	remoteRefs, err := s.discoverPullRequestActorRefs(forkURL, cfg.Mainline.ActorLogPrefix, "")
	if err != nil {
		return nil, err
	}
	sort.SliceStable(remoteRefs, func(i, j int) bool {
		if remoteRefs[i].ActorID != remoteRefs[j].ActorID {
			return remoteRefs[i].ActorID < remoteRefs[j].ActorID
		}
		return remoteRefs[i].SourceRef < remoteRefs[j].SourceRef
	})

	byIntent := make(map[string]domain.IntentView)
	conflicted := make(map[string]bool)
	for _, remoteRef := range remoteRefs {
		intents, err := s.readForkActorPRCommentIntents(forkURL, remoteRef, prNumber)
		if err != nil {
			return nil, err
		}
		for _, intent := range intents {
			if conflicted[intent.IntentID] {
				continue
			}
			if existing, ok := byIntent[intent.IntentID]; ok && existing.ActorID != intent.ActorID {
				delete(byIntent, intent.IntentID)
				conflicted[intent.IntentID] = true
				continue
			}
			byIntent[intent.IntentID] = intent
		}
	}

	out := make([]domain.IntentView, 0, len(byIntent))
	for _, intent := range byIntent {
		out = append(out, intent)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ActorID != out[j].ActorID {
			return out[i].ActorID < out[j].ActorID
		}
		return out[i].IntentID < out[j].IntentID
	})
	return out, nil
}

func (s *Service) readForkActorPRCommentIntents(
	forkURL string,
	remoteRef pullRequestActorRef,
	prNumber int,
) ([]domain.IntentView, error) {
	importRef := forkPRCommentImportRef(remoteRef.ActorID, prNumber)
	defer func() {
		_, _ = s.Git.Run("update-ref", "-d", importRef)
	}()

	refspec := "+" + remoteRef.SourceRef + ":" + importRef
	if err := s.Git.Fetch(forkURL, refspec); err != nil {
		return nil, domain.NewRecoverableError(
			domain.ErrSyncFailed,
			fmt.Sprintf("fetch actor log %s from %s failed: %v", remoteRef.SourceRef, forkURL, err),
			"check the fork URL",
			"ask the contributor to run mainline publish --remote <fork>",
		)
	}
	if actual := s.Git.ReadRef(importRef); actual != remoteRef.SourceHead {
		return nil, domain.NewRecoverableError(
			domain.ErrSyncFailed,
			fmt.Sprintf("fork actor log %s changed while preparing the PR comment", remoteRef.SourceRef),
			"retry the PR comment workflow against the latest fork state",
		)
	}

	rawEvents, err := s.Store.ReadActorLogEventsFromRef(importRef)
	if err != nil {
		return nil, domain.NewRecoverableError(
			domain.ErrSyncFailed,
			fmt.Sprintf("read actor log %s failed: %v", remoteRef.SourceRef, err),
			"retry the PR comment workflow",
		)
	}

	intents := make(map[string]domain.IntentView)
	for index, raw := range rawEvents {
		var base domain.BaseEvent
		if err := json.Unmarshal(raw, &base); err != nil {
			return nil, domain.NewError(
				domain.ErrInvalidInput,
				fmt.Sprintf("fork actor log event %d is not valid JSON: %v", index, err),
			)
		}
		if base.ActorID != remoteRef.ActorID {
			continue
		}

		switch base.EventType {
		case domain.EventIntentSealed:
			var event domain.IntentSealedEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				return nil, domain.NewError(
					domain.ErrInvalidInput,
					fmt.Sprintf("fork sealed event %s is invalid: %v", base.EventID, err),
				)
			}
			if strings.TrimSpace(event.IntentID) == "" {
				continue
			}
			intents[event.IntentID] = forkPRIntentView(event, forkURL, remoteRef)
		case domain.EventIntentAbandoned:
			var event domain.IntentAbandonedEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				return nil, domain.NewError(
					domain.ErrInvalidInput,
					fmt.Sprintf("fork abandoned event %s is invalid: %v", base.EventID, err),
				)
			}
			if intent, ok := intents[event.IntentID]; ok {
				intent.Status = domain.StatusAbandoned
				intents[event.IntentID] = intent
			}
		case domain.EventIntentSuperseded:
			var event domain.IntentSupersededEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				return nil, domain.NewError(
					domain.ErrInvalidInput,
					fmt.Sprintf("fork superseded event %s is invalid: %v", base.EventID, err),
				)
			}
			if intent, ok := intents[event.IntentID]; ok {
				intent.Status = domain.StatusSuperseded
				intents[event.IntentID] = intent
			}
		}
	}

	out := make([]domain.IntentView, 0, len(intents))
	for _, intent := range intents {
		out = append(out, intent)
	}
	return out, nil
}

func forkPRIntentView(
	event domain.IntentSealedEvent,
	forkURL string,
	remoteRef pullRequestActorRef,
) domain.IntentView {
	summary := event.Summary
	summary.UserGoal = event.Goal

	worktreeStatus := event.WorktreeStatus
	if worktreeStatus == "" {
		worktreeStatus = "clean"
	}
	sealedAtBranch := event.SealedAtBranch
	if sealedAtBranch == "" {
		sealedAtBranch = event.GitBranch
	}
	evidenceComplete := event.EvidenceComplete
	if event.WorktreeStatus == "" {
		evidenceComplete = true
	}

	return domain.IntentView{
		IntentID:        event.IntentID,
		SchemaVersion:   1,
		Status:          domain.StatusProposed,
		Publication:     "published",
		ActorID:         event.ActorID,
		ActorName:       event.ActorName,
		Thread:          event.Thread,
		GitBranch:       event.GitBranch,
		Goal:            event.Goal,
		SealedAt:        event.SealedAt,
		BaseCommit:      event.BaseCommit,
		CodeCommit:      event.CodeCommit,
		CodeTree:        event.CodeTree,
		BackfillCommits: event.BackfillCommits,
		Summary:         &summary,
		Fingerprint:     &event.Fingerprint,
		References:      event.References,
		StatusEvidence: domain.StatusEvidence{
			SealedEventID:    event.EventID,
			EvidenceComplete: evidenceComplete,
			WorktreeStatus:   worktreeStatus,
			SealedAtBranch:   sealedAtBranch,
		},
		Provenance: &domain.IntentProvenance{
			Kind:         forkPRCommentProvenanceKind,
			SourceRemote: forkURL,
			SourceRef:    remoteRef.SourceRef,
			SourceHead:   remoteRef.SourceHead,
		},
	}
}

func forkPRCommentImportRef(actorID string, prNumber int) string {
	if prNumber > 0 {
		return fmt.Sprintf("refs/mainline/imports/pr-comments/pr-%d/%s/log", prNumber, actorID)
	}
	return "refs/mainline/imports/pr-comments/manual/" + actorID + "/log"
}

func mergePRCommentIntents(upstream, fork []domain.IntentView) []domain.IntentView {
	out := make([]domain.IntentView, 0, len(upstream)+len(fork))
	seen := make(map[string]bool, len(upstream)+len(fork))
	for _, intent := range upstream {
		out = append(out, intent)
		seen[intent.IntentID] = true
	}
	for _, intent := range fork {
		if seen[intent.IntentID] {
			continue
		}
		out = append(out, intent)
		seen[intent.IntentID] = true
	}
	return out
}
