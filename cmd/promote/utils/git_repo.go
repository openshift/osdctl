package utils

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/storage/memory"
)

type Repo struct {
	url     string
	rawRepo *git.Repository
}

func GetRepo(repoURL string) (*Repo, error) {
	rawRepo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: repoURL,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to clone '%s' in memory: %v", repoURL, err)
	}

	return &Repo{repoURL, rawRepo}, err
}

func (r *Repo) GetURL() string {
	return r.url
}

func (r *Repo) GetHeadHash() (string, error) {
	head, err := r.rawRepo.Head()

	if err != nil {
		return "", fmt.Errorf("failed to read HEAD commit for '%s': %v", r.url, err)
	}

	return head.Hash().String(), nil
}

func (r *Repo) FormattedLog(fileRelPath, commonAncestorHash, targetHash string) (string, error) {
	fileRelPath = strings.TrimPrefix(fileRelPath, string(filepath.Separator))
	commonAncestorCommit, err := r.rawRepo.CommitObject(plumbing.NewHash(commonAncestorHash))
	if commonAncestorCommit == nil || err != nil {
		return "", fmt.Errorf("common ancestor commit '%s' does not exist in '%s': %v", commonAncestorHash, r.url, err)
	}

	var sb strings.Builder
	queue := []plumbing.Hash{plumbing.NewHash(targetHash)}
	visitedHashes := make(map[plumbing.Hash]struct{})
	visitedHashes[commonAncestorCommit.Hash] = struct{}{}

	for len(queue) > 0 {
		idx := len(queue) - 1
		hash := queue[idx]
		queue = queue[:idx]

		_, visited := visitedHashes[hash]
		if visited {
			continue
		}

		visitedHashes[hash] = struct{}{}
		commit, err := r.rawRepo.CommitObject(hash)
		if commit == nil || err != nil {
			return "", fmt.Errorf("commit '%s' does not exist in '%s': %v", hash.String(), r.url, err)
		}

		isAncestor, err := commit.IsAncestor(commonAncestorCommit)
		if err != nil {
			return "", fmt.Errorf("unable to determine if commit '%s' is ancestor of '%s' in '%s': %v", hash.String(), commonAncestorHash, r.url, err)
		}

		if !isAncestor {
			if len(commit.ParentHashes) < 2 {
				_, err := commit.File(fileRelPath)
				if err == nil {
					sb.WriteString(commit.String())
				}
			}

			queue = append(queue, commit.ParentHashes...)
		}
	}

	return sb.String(), nil
}
