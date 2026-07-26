package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mainline-org/mainline/internal/domain"
)

func TestPRCommentWithOptionsReadsForkIntentWithoutImporting(t *testing.T) {
	dir, cleanup := testRepo(t)
	defer cleanup()

	svc := NewServiceFromRoot(dir)
	if _, err := svc.Init("maintainer"); err != nil {
		t.Fatalf("init: %v", err)
	}

	forkDir := cloneForkForPRImport(t, dir)
	branch := "feature/fork-pr-comment"
	codeCommit, codeTree := seedForkPRBranch(t, forkDir, branch, "sources/fork_pr_comment.go")
	actorID := "actor_fork_pr_comment"
	intentID := "int_fork_pr_comment"
	writeForkActorLog(t, svc, forkDir, actorID, intentID, branch, codeCommit, codeTree)
	fetchForkPRHeadForComment(t, svc, forkDir, branch, 203)

	beforeView, _ := svc.Store.ReadMainlineView()
	beforeViewJSON, _ := json.Marshal(beforeView)
	beforeNotes := svc.Git.ReadRef("refs/notes/mainline/intents")
	sourceRef := domain.ActorLogRef(actorID, domain.DefaultActorLogPrefix)
	beforeSourceHead := strings.TrimSpace(mustGitRun(t, forkDir, "rev-parse", sourceRef))

	comment, err := svc.PRCommentWithOptions(PullRequestCommentOptions{
		Base:     svc.Git.ReadRef("refs/heads/main"),
		Head:     codeCommit,
		Branch:   branch,
		PRNumber: 203,
		ForkURL:  forkDir,
	})
	if err != nil {
		t.Fatalf("fork PR comment: %v", err)
	}
	for _, want := range []string{
		prCommentMarker,
		intentID,
		"Fork PR import fixture",
		"Contributor-published intent; not yet accepted into the upstream Mainline log.",
	} {
		if !strings.Contains(comment, want) {
			t.Fatalf("fork comment missing %q:\n%s", want, comment)
		}
	}

	if got := svc.Git.ReadRef(sourceRef); got != "" {
		t.Fatalf("PR comment must not accept the fork actor ref, got %s", got)
	}
	if got := svc.Git.ReadRef("refs/notes/mainline/intents"); got != beforeNotes {
		t.Fatalf("PR comment changed Mainline notes: before=%s after=%s", beforeNotes, got)
	}
	afterView, _ := svc.Store.ReadMainlineView()
	afterViewJSON, _ := json.Marshal(afterView)
	if string(afterViewJSON) != string(beforeViewJSON) {
		t.Fatalf("PR comment changed the upstream Mainline view")
	}
	if got := strings.TrimSpace(mustGitRun(t, forkDir, "rev-parse", sourceRef)); got != beforeSourceHead {
		t.Fatalf("PR comment changed the fork actor ref: before=%s after=%s", beforeSourceHead, got)
	}
	assertNoForkPRCommentImportRefs(t, svc)
}

func TestPRCommentWithOptionsRendersMultipleForkIntentsAndFiltersTerminalState(t *testing.T) {
	dir, cleanup := testRepo(t)
	defer cleanup()

	svc := NewServiceFromRoot(dir)
	if _, err := svc.Init("maintainer"); err != nil {
		t.Fatalf("init: %v", err)
	}

	forkDir := cloneForkForPRImport(t, dir)
	branch := "feature/fork-pr-comment-multiple"
	gitCmd(t, forkDir, "checkout", "-b", branch, "main")
	writeFile(t, forkDir, "sources/fork_pr_comment_first.go", "package sources\n")
	gitCmd(t, forkDir, "add", "sources/fork_pr_comment_first.go")
	gitCmd(t, forkDir, "commit", "-m", "feat: add first fork PR comment fixture")
	firstCommit := strings.TrimSpace(mustGitRun(t, forkDir, "rev-parse", "HEAD"))
	writeFile(t, forkDir, "sources/fork_pr_comment_second.go", "package sources\n")
	gitCmd(t, forkDir, "add", "sources/fork_pr_comment_second.go")
	gitCmd(t, forkDir, "commit", "-m", "feat: add second fork PR comment fixture")
	headCommit := strings.TrimSpace(mustGitRun(t, forkDir, "rev-parse", "HEAD"))

	writeForkPRCommentIntent(t, forkDir, "actor_fork_comment_first", "int_fork_comment_first",
		branch, firstCommit, "2026-07-01T00:00:00Z", "First fork intent")
	writeForkPRCommentIntent(t, forkDir, "actor_fork_comment_second", "int_fork_comment_second",
		branch, headCommit, "2026-07-02T00:00:00Z", "Second fork intent")
	writeForkPRCommentIntent(t, forkDir, "actor_fork_comment_terminal", "int_fork_comment_terminal",
		branch, headCommit, "2026-07-03T00:00:00Z", "Abandoned fork intent")
	appendForkActorEvent(t, forkDir, "actor_fork_comment_terminal", domain.IntentAbandonedEvent{
		BaseEvent: domain.BaseEvent{
			EventID:       "evt_fork_comment_terminal_abandoned",
			SchemaVersion: 1,
			EventType:     domain.EventIntentAbandoned,
			ActorID:       "actor_fork_comment_terminal",
			ActorName:     "fork contributor",
			Timestamp:     "2026-07-04T00:00:00Z",
		},
		IntentID: "int_fork_comment_terminal",
		Reason:   "superseded by the submitted implementation",
	})
	fetchForkPRHeadForComment(t, svc, forkDir, branch, 204)

	comment, err := svc.PRCommentWithOptions(PullRequestCommentOptions{
		Base:     svc.Git.ReadRef("refs/heads/main"),
		Head:     headCommit,
		Branch:   branch,
		PRNumber: 204,
		ForkURL:  forkDir,
	})
	if err != nil {
		t.Fatalf("fork PR comment: %v", err)
	}
	firstIndex := strings.Index(comment, "First fork intent")
	secondIndex := strings.Index(comment, "Second fork intent")
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("fork intents were not rendered in sealed order:\n%s", comment)
	}
	if strings.Contains(comment, "Abandoned fork intent") {
		t.Fatalf("terminal fork intent must not be rendered:\n%s", comment)
	}
	if strings.Count(comment, "### Mainline Intent") != 2 {
		t.Fatalf("expected two fork intents, got:\n%s", comment)
	}
	assertNoForkPRCommentImportRefs(t, svc)
}

func TestPRCommentWithOptionsIgnoresMismatchedForkActorEvent(t *testing.T) {
	dir, cleanup := testRepo(t)
	defer cleanup()

	svc := NewServiceFromRoot(dir)
	if _, err := svc.Init("maintainer"); err != nil {
		t.Fatalf("init: %v", err)
	}

	forkDir := cloneForkForPRImport(t, dir)
	branch := "feature/fork-pr-comment-mismatch"
	codeCommit, _ := seedForkPRBranch(t, forkDir, branch, "sources/fork_pr_comment_mismatch.go")
	sourceActor := "actor_fork_comment_source"
	event := forkPRCommentSealedEvent(
		"actor_fork_comment_other",
		"int_fork_comment_mismatch",
		branch,
		codeCommit,
		"2026-07-01T00:00:00Z",
		"Mismatched fork intent",
	)
	sourceRef := domain.ActorLogRef(sourceActor, domain.DefaultActorLogPrefix)
	forkSvc := NewServiceFromRoot(forkDir)
	if err := forkSvc.Git.UpdateRef(sourceRef, writeActorEventCommit(t, forkSvc, event)); err != nil {
		t.Fatalf("write mismatched fork actor ref: %v", err)
	}
	fetchForkPRHeadForComment(t, svc, forkDir, branch, 205)

	comment, err := svc.PRCommentWithOptions(PullRequestCommentOptions{
		Base:     svc.Git.ReadRef("refs/heads/main"),
		Head:     codeCommit,
		Branch:   branch,
		PRNumber: 205,
		ForkURL:  forkDir,
	})
	if err != nil {
		t.Fatalf("fork PR comment: %v", err)
	}
	if !strings.Contains(comment, "No sealed Mainline intent was found for this PR range.") {
		t.Fatalf("mismatched actor event should produce the existing missing comment:\n%s", comment)
	}
	if strings.Contains(comment, "Mismatched fork intent") {
		t.Fatalf("mismatched actor event must not be rendered:\n%s", comment)
	}
	assertNoForkPRCommentImportRefs(t, svc)
}

func writeForkPRCommentIntent(
	t *testing.T,
	forkDir, actorID, intentID, branch, codeCommit, sealedAt, title string,
) {
	t.Helper()
	appendForkActorEvent(t, forkDir, actorID,
		forkPRCommentSealedEvent(actorID, intentID, branch, codeCommit, sealedAt, title))
}

func forkPRCommentSealedEvent(
	actorID, intentID, branch, codeCommit, sealedAt, title string,
) domain.IntentSealedEvent {
	return domain.IntentSealedEvent{
		BaseEvent: domain.BaseEvent{
			EventID:       "evt_" + intentID + "_sealed",
			SchemaVersion: 1,
			EventType:     domain.EventIntentSealed,
			ActorID:       actorID,
			ActorName:     "fork contributor",
			Timestamp:     sealedAt,
		},
		IntentID:   intentID,
		Thread:     branch,
		Goal:       "exercise fork PR intent comments",
		GitBranch:  branch,
		CodeCommit: codeCommit,
		Summary: domain.IntentSummary{
			Title:    title,
			What:     "Rendered contributor-published intent metadata without importing it.",
			Why:      "Fork PR reviewers need the same Mainline context as same-repository PR reviewers.",
			UserGoal: "exercise fork PR intent comments",
		},
		Fingerprint: domain.SemanticFingerprint{
			Subsystems:   []string{"fork-pr-comment"},
			FilesTouched: []string{"sources"},
		},
		TurnCount: 1,
		SealedAt:  sealedAt,
	}
}

func appendForkActorEvent(t *testing.T, forkDir, actorID string, event any) {
	t.Helper()
	forkSvc := NewServiceFromRoot(forkDir)
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal fork actor event: %v", err)
	}
	blobHash, err := forkSvc.Git.HashObject(data)
	if err != nil {
		t.Fatalf("hash fork actor event: %v", err)
	}
	treeHash, err := forkSvc.Git.MakeTree("event.json", blobHash)
	if err != nil {
		t.Fatalf("make fork actor event tree: %v", err)
	}
	sourceRef := domain.ActorLogRef(actorID, domain.DefaultActorLogPrefix)
	parent := forkSvc.Git.ReadRef(sourceRef)
	commitHash, err := forkSvc.Git.CommitTree(treeHash, parent, "actor-log-event")
	if err != nil {
		t.Fatalf("commit fork actor event: %v", err)
	}
	if err := forkSvc.Git.UpdateRef(sourceRef, commitHash); err != nil {
		t.Fatalf("update fork actor ref: %v", err)
	}
}

func fetchForkPRHeadForComment(
	t *testing.T,
	svc *Service,
	forkDir, branch string,
	prNumber int,
) {
	t.Helper()
	refspec := fmt.Sprintf(
		"+refs/heads/%s:refs/mainline/imports/pr-heads/pr-%d",
		branch,
		prNumber,
	)
	if err := svc.Git.Fetch(forkDir, refspec); err != nil {
		t.Fatalf("fetch fork PR head: %v", err)
	}
}

func assertNoForkPRCommentImportRefs(t *testing.T, svc *Service) {
	t.Helper()
	refs, err := svc.Git.ListRefs("refs/mainline/imports/pr-comments")
	if err != nil {
		t.Fatalf("list fork PR comment refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("fork PR comment refs were not cleaned up: %v", refs)
	}
}
