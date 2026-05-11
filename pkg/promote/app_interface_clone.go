package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type AppInterfaceClone struct {
	path     string
	repo     *git.Repository
	workTree *git.Worktree
}

func getDefaultAppInterfacePath() string {
	return filepath.Join(os.Getenv("HOME"), "git", "app-interface")
}

func hasAppInterfaceRemote(remotes []*git.Remote) bool {
	for _, remote := range remotes {
		for _, remoteURL := range remote.Config().URLs {
			if strings.Contains(remoteURL, "gitlab.cee.redhat.com") && strings.Contains(remoteURL, "app-interface") {
				return true
			}
		}
	}

	return false
}

func newAppInterfaceClone(path string, repo *git.Repository) (*AppInterfaceClone, error) {
	var err error

	if repo == nil {
		repo, err = git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})

		if err != nil {
			return nil, fmt.Errorf("'%s' is not a git repository: %v", path, err)
		}
	}

	workTree, err := repo.Worktree()
	if err != nil || workTree == nil {
		return nil, fmt.Errorf("'%s' does not come with a work tree: %v", path, err)
	}

	path = workTree.Filesystem.Root()

	{
		remotes, err := repo.Remotes()

		if err != nil {
			return nil, fmt.Errorf("'%s' remotes cannot be listed: %v", path, err)
		}

		if !hasAppInterfaceRemote(remotes) {
			return nil, fmt.Errorf("'%s' does not have an app-interface remote", path)
		}
	}

	return &AppInterfaceClone{path, repo, workTree}, nil
}

func FindAppInterfaceClone(providedPath string) (*AppInterfaceClone, error) {
	if providedPath != "" {
		appInterfaceClone, err := newAppInterfaceClone(providedPath, nil)
		if err != nil {
			return nil, fmt.Errorf("provided directory '%s' is not a workable app-interface clone location: %v", providedPath, err)
		}
		return appInterfaceClone, nil
	}

	{
		currentDirPath, err := os.Getwd()
		if err == nil {
			var currentDirRepo *git.Repository

			currentDirRepo, err = git.PlainOpenWithOptions(currentDirPath, &git.PlainOpenOptions{DetectDotGit: true})

			if err == nil {
				var appInterfaceClone *AppInterfaceClone

				appInterfaceClone, err = newAppInterfaceClone(currentDirPath, currentDirRepo)
				if err == nil {
					return appInterfaceClone, nil
				}
			}
		}
		fmt.Printf("Current working directory does not appear to be a suitable app-interface clone location: %v\n\n", err)
	}

	{
		defaultAppInterfacePath := getDefaultAppInterfacePath()
		appInterfaceClone, err := newAppInterfaceClone(defaultAppInterfacePath, nil)
		if err != nil {
			return nil, fmt.Errorf("default directory '%s' is not an app-interface clone location: %v", defaultAppInterfacePath, err)
		}
		return appInterfaceClone, nil
	}
}

func (a *AppInterfaceClone) GetPath() string {
	return a.path
}

func (a *AppInterfaceClone) IsClean() (bool, error) {
	status, err := a.workTree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get the status of the work tree of '%s': %v", a.path, err)
	}

	return status.IsClean(), nil
}

func (a *AppInterfaceClone) CheckoutNewBranch(branchName string) error {
	if err := a.workTree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master")}); err != nil {
		return fmt.Errorf("failed to checkout master branch in '%s': %v", a.path, err)
	}

	branchReference := plumbing.NewBranchReferenceName(branchName)

	if branch, err := a.repo.Reference(branchReference, true); branch != nil && err == nil {
		if err := a.repo.Storer.RemoveReference(branchReference); err != nil {
			return fmt.Errorf("failed to delete '%s' branch in '%s': %v", branchName, a.path, err)
		}
	}

	if err := a.workTree.Checkout(&git.CheckoutOptions{Branch: branchReference, Create: true}); err != nil {
		return fmt.Errorf("failed to create and checkout '%s' branch in '%s': %v", branchName, a.path, err)
	}

	return nil
}

func (a *AppInterfaceClone) Commit(commitMessage string) error {
	if err := a.workTree.AddGlob("."); err != nil {
		return fmt.Errorf("failed to add files to the git index : %v", err)
	}

	if _, err := a.workTree.Commit(commitMessage, &git.CommitOptions{}); err != nil {
		return fmt.Errorf("failed to commit changes in '%s': %v", a.path, err)
	}

	return nil
}
