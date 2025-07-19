package llmcontext

import (
	"context"
	"sync"
	"time"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/logze/v2"
)

type repoDataProvider struct {
	provider interfaces.CodeProvider

	mr           *model.MergeRequest
	diffs        []*model.FileDiff
	allComments  []*model.Comment
	repoInfo     *model.RepositoryInfo
	repoDataHead *model.RepositorySnapshot
	repoDataBase *model.RepositorySnapshot

	commits     []*model.Commit
	commitsDiff []*model.FileDiff

	isVerbose bool

	log logze.Logger

	mu sync.Mutex
}

func newRepoDataProvider(provider interfaces.CodeProvider, isVerbose bool) *repoDataProvider {
	return &repoDataProvider{provider: provider, log: logze.With("Module", "repo_data_provider"), isVerbose: isVerbose}
}

func (r *repoDataProvider) loadMRData(ctx context.Context, projectID string, mrIID int) error {
	waiterSet := abstract.NewWaiterSet(r.log)
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadMR(ctx, projectID, mrIID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadDiffs(ctx, projectID, mrIID)
	})
	err := waiterSet.Await(ctx)
	if err != nil {
		return erro.Wrap(err, "failed to load repository data")
	}

	return nil
}

func (r *repoDataProvider) loadAllData(ctx context.Context, projectID string, mrIID int) error {
	waiterSet := abstract.NewWaiterSet(r.log)
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadMR(ctx, projectID, mrIID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadRepoInfo(ctx, projectID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadDiffs(ctx, projectID, mrIID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadAllComments(ctx, projectID, mrIID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadRepoDataHead(ctx, projectID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadRepoDataBase(ctx, projectID)
	})
	waiterSet.Add(ctx, func(ctx context.Context) error {
		return r.loadCommitsDiff(ctx, projectID, mrIID)
	})
	err := waiterSet.Await(ctx)
	if err != nil {
		return erro.Wrap(err, "failed to load repository data")
	}

	return nil
}

func (r *repoDataProvider) loadMR(ctx context.Context, projectID string, mrIID int) error {
	if r.mr != nil {
		return nil
	}
	timer := abstract.StartTimer()
	mr, err := r.provider.GetMergeRequest(ctx, projectID, mrIID)
	if err != nil {
		return erro.Wrap(err, "failed to get merge request")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mr = mr
	r.log.DebugIf(r.isVerbose, "loaded merge request", "mr", mrIID, "elapsed", timer.ElapsedTime().String())
	return nil
}

func (r *repoDataProvider) loadDiffs(ctx context.Context, projectID string, mrIID int) error {
	if r.diffs != nil {
		return nil
	}
	timer := abstract.StartTimer()
	diffs, err := r.provider.GetMergeRequestDiffs(ctx, projectID, mrIID)
	if err != nil {
		return erro.Wrap(err, "failed to get MR diffs")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.diffs = diffs
	r.log.DebugIf(r.isVerbose, "loaded merge request diffs", "mr", mrIID, "elapsed", timer.ElapsedTime().String())
	return nil
}

func (r *repoDataProvider) loadCommitsDiff(ctx context.Context, projectID string, mrIID int) error {
	timer := abstract.StartTimer()
	commits, err := r.provider.GetMergeRequestCommits(ctx, projectID, mrIID)
	if err != nil {
		return erro.Wrap(err, "failed to get merge request commits")
	}
	var commitsDiff []*model.FileDiff
	for _, commit := range commits {
		fileDiffs, err := r.provider.GetCommitDiffs(ctx, projectID, commit.SHA)
		if err != nil {
			return erro.Wrap(err, "failed to get commit diffs")
		}
		commitsDiff = append(commitsDiff, fileDiffs...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commits = commits
	r.commitsDiff = commitsDiff
	r.log.DebugIf(r.isVerbose, "loaded merge request commits diffs", "mr", mrIID, "elapsed", timer.ElapsedTime().String())
	return nil
}

func (r *repoDataProvider) loadAllComments(ctx context.Context, projectID string, mrIID int) error {
	timer := abstract.StartTimer()
	comments, err := r.provider.GetComments(ctx, projectID, mrIID)
	if err != nil {
		return erro.Wrap(err, "failed to get comments")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allComments = comments
	r.log.DebugIf(r.isVerbose, "loaded merge request comments", "mr", mrIID, "elapsed", timer.ElapsedTime().String())
	return nil
}

func (r *repoDataProvider) loadRepoInfo(ctx context.Context, projectID string) error {
	timer := abstract.StartTimer()
	repoInfo, err := r.provider.GetRepositoryInfo(ctx, projectID)
	if err != nil {
		return erro.Wrap(err, "failed to get repository info")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.repoInfo = repoInfo
	r.log.DebugIf(r.isVerbose, "loaded repository info", "repo", repoInfo.Name, "elapsed", timer.ElapsedTime().String())
	return nil
}

func (r *repoDataProvider) loadRepoDataHead(ctx context.Context, projectID string) error {
	timer := abstract.StartTimer()
	for r.mr == nil {
		time.Sleep(100 * time.Millisecond)
	}
	repoDataHead, err := r.provider.GetRepositorySnapshot(ctx, projectID, r.mr.SHA)
	if err != nil {
		return erro.Wrap(err, "failed to get repository data (head)")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.repoDataHead = repoDataHead
	r.log.DebugIf(r.isVerbose, "loaded repository data (head)", "mr", r.mr.IID, "elapsed", timer.ElapsedTime().String())
	return nil
}

func (r *repoDataProvider) loadRepoDataBase(ctx context.Context, projectID string) error {
	timer := abstract.StartTimer()
	for r.repoInfo == nil {
		time.Sleep(100 * time.Millisecond)
	}
	for r.mr == nil {
		time.Sleep(100 * time.Millisecond)
	}
	for _, branch := range r.repoInfo.Branches {
		if branch.Name == r.mr.TargetBranch {
			repoDataBase, err := r.provider.GetRepositorySnapshot(ctx, projectID, branch.SHA)
			if err != nil {
				return erro.Wrap(err, "failed to get repository data")
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			r.repoDataBase = repoDataBase
			r.log.DebugIf(r.isVerbose, "loaded repository data (base)", "mr", r.mr.IID, "elapsed", timer.ElapsedTime().String())
			return nil
		}
	}
	return erro.New("failed to find repository data for base branch")
}
