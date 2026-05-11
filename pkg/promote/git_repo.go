package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

type Repo struct {
	url       string
	clonePath string
	rawRepo   *git.Repository
}

func GetRepo(repoUrl string) (*Repo, error) {
	// For performance reasons, it is not suitable to have a in-memory clone using go-git:
	//
	// rawRepo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
	//     URL:    repoUrl,
	//     Mirror: true,
	// })
	//
	// Doing so will consume up to 30 GB of memory on repositories like managed-cluster-config which are a few GB on disk. Moreover, the 'Log' method of the in-memory clone is very slow (it can take up to 10 minutes to execute 'Log' on the in-memory clone of managed-cluster-config while it takes less than 10 seconds on a disk clone).
	// Hence we are cloning the repository in the temp directory created below:

	clonePath, err := os.MkdirTemp("", "promote-clone")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory to clone '%s': %v", repoUrl, err)
	}
	removeClonePath := true
	defer func() {
		if removeClonePath {
			err := os.RemoveAll(clonePath)
			if err != nil {
				fmt.Printf("Warning: Failed to remove clone directory '%s': %v", clonePath, err)
			}
		}
	}()

	rawRepo, err := git.PlainClone(clonePath, false, &git.CloneOptions{
		URL:    repoUrl,
		Mirror: true,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to clone '%s': %v", repoUrl, err)
	}

	// Also remark that calling the git command line is way faster (4 times faster):
	//
	// cmd := exec.Command("git", "clone", "--mirror", repoUrl, clonePath)
	// cmd.Dir = clonePath
	// err := cmd.Run()
	//
	// rawRepo, err := git.PlainOpen(clonePath)
	//
	// But this is also less portable (git CLI must be installed... and it does not behave the same on all platforms)
	// Also the time it takes to clone a big repository (few GBs) is still acceptable with go-git (less than 1  minute)

	removeClonePath = false

	return &Repo{url: repoUrl, clonePath: clonePath, rawRepo: rawRepo}, nil
}

func (r *Repo) Cleanup() {
	err := os.RemoveAll(r.clonePath)
	if err != nil {
		fmt.Printf("Warning:Failed to remove clone directory '%s': %v", r.clonePath, err)
	}
}

func (r *Repo) GetUrl() string {
	return r.url
}

func (r *Repo) GetHeadHash() (string, error) {
	head, err := r.rawRepo.Head()

	if err != nil {
		return "", fmt.Errorf("failed to read HEAD commit for '%s': %v", r.url, err)
	}

	return head.Hash().String(), nil
}

func (r *Repo) ResolveHash(hashOrBranchName string) string {
	branchesIt, err := r.rawRepo.Branches()
	if err != nil {
		fmt.Printf("Warning: Failed to list branches for '%s': %v\n", r.url, err)
		return hashOrBranchName
	}

	_ = branchesIt.ForEach(func(branch *plumbing.Reference) error {
		if branch.Name().Short() == hashOrBranchName {
			hashOrBranchName = branch.Hash().String()
			return storer.ErrStop
		}
		return nil
	})

	return hashOrBranchName
}

func (r *Repo) FormattedLog(fileRelPath, commonAncestorHash, targetHash string) (string, error) {
	// Be aware that calling 'Log' as follows is not equivalent to calling 'git log commonAncestorHash..targetHash -- fileRelPath':
	// (the 'Log' method only returns the descendants of 'commonAncestorHash' while 'git log' returns all the commits which are not ancestor of  'commonAncestorHash')
	//
	// r.rawRepo.Log(&git.LogOptions{
	//     From:  plumbing.NewHash(targetHash),
	// 	   To:    plumbing.NewHash(commonAncestorHash),
	// 	   Order: git.LogOrderBSF,
	// 	})
	//
	// Hence the below code which is a bit complex but equally fast to the 'Log' method.

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

		isAncestor := false
		isDescendant, err := commonAncestorCommit.IsAncestor(commit) // This is fast to compute as the whole tree does not have to be traversed
		if err != nil {
			return "", fmt.Errorf("unable to determine if commit '%s' is descendant of '%s' in '%s': %v", hash.String(), commonAncestorHash, r.url, err)
		}
		if !isDescendant {
			isAncestor, err = commit.IsAncestor(commonAncestorCommit)
			if err != nil {
				return "", fmt.Errorf("unable to determine if commit '%s' is ancestor of '%s' in '%s': %v", hash.String(), commonAncestorHash, r.url, err)
			}
		}

		if !isAncestor {
			if len(commit.ParentHashes) < 2 {
				stats, err := commit.Stats()
				if err != nil {
					return "", fmt.Errorf("unable to get stats for commit '%s' in '%s': %v", hash.String(), r.url, err)
				}
				for _, stat := range stats {
					if stat.Name == fileRelPath {
						sb.WriteString(commit.String())
					}
				}
			}

			queue = append(queue, commit.ParentHashes...)
		}
	}

	return sb.String(), nil
}
