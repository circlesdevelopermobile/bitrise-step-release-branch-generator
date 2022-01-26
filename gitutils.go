package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

func gitClone(url string, path string, branch string, auth transport.AuthMethod) (*git.Repository, error) {
	refName := gitRefName(branch)
	repo, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:           url,
		Auth:          auth,
		ReferenceName: refName,
		Progress:      os.Stdout,
		Tags:          git.AllTags,
	})
	return repo, err
}

func gitRefName(name string) plumbing.ReferenceName {
	return plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", name))
}

func gitCheckoutBranch(repo *git.Repository, branchName string) error {
	branch := gitRefName(branchName)
	wt, _ := repo.Worktree()
	err := wt.Checkout(&git.CheckoutOptions{
		Branch: branch,
	})
	if err != nil {
		return err
	}
	return nil
}

func gitAddAll(repo *git.Repository) error {
	wt, _ := repo.Worktree()
	err := wt.AddGlob(".")
	if err != nil {
		return err
	}
	return nil
}

func gitCommit(repo *git.Repository, commitMsg string) error {
	wt, _ := repo.Worktree()
	_, err := wt.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Bitrise",
			Email: "bitrise@bitrise.io",
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func gitTag(repo *git.Repository, tagName string) error {
	head, _ := repo.Head()
	_, _ = fmt.Fprintf(os.Stdout, "Attempting to tag HEAD with: %s\n", tagName)
	_, err := repo.CreateTag(tagName, head.Hash(), nil)

	if err != nil {
		return errors.New(fmt.Sprintf("error creating tag: %v\n", err))
	}
	return nil
}

func gitPushTag(repo *git.Repository, auth transport.AuthMethod, tagName string) error {
	fmt.Fprintf(os.Stdout, "Attempting to push tag %s\n", tagName)
	refSpec := config.RefSpec("refs/tags/*:refs/tags/*")
	if tagName != "" {
		refSpec = config.RefSpec(fmt.Sprintf("refs/tags/%[1]s:refs/tags/%[1]s", tagName))
	}
	opts := git.PushOptions{
		RefSpecs: []config.RefSpec{refSpec},
		Progress: os.Stdout,
		Auth:     auth,
	}
	err := repo.Push(&opts)
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	}
	return nil
}

func gitPushBranch(repo *git.Repository, auth transport.AuthMethod, branchName string) error {
	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%[1]s:refs/heads/%[1]s", branchName))
	opts := git.PushOptions{
		RefSpecs: []config.RefSpec{refSpec},
		Progress: os.Stdout,
		Auth:     auth,
	}
	err := repo.Push(&opts)
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return errors.New(fmt.Sprintf("unable to push branch: %v\n", err))
	}
	return nil
}

func getGitAuth(cfg *Config) (transport.AuthMethod, error) {
	if strings.HasPrefix(cfg.CloneUrl, "http") {
		auth := &http.BasicAuth{
			Username: cfg.Username,
			Password: string(cfg.AccessToken),
		}
		return auth, nil
	} else {
		sshPk, err := ssh.NewPublicKeysFromFile("git", cfg.SSHPrivateKeyPath, "")
		if err != nil {
			return nil, err
		}
		return sshPk, err
	}
}

func processTagFile(repo *git.Repository, auth transport.AuthMethod, config *Config) error {
	file, _ := os.OpenFile(config.tagFilePath(), os.O_RDONLY, 0644)
	defer file.Close()
	reader := bufio.NewScanner(file)

	var tags []string
	var tagsToPush []string

	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if !strings.HasPrefix(line, "#") && line != "" {
			tags = append(tags, line)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	for _, tag := range tags {

		// Append suffix to tag parsed from tag file
		var sb strings.Builder
		sb.WriteString(tag)
		sb.WriteString(config.TagNameSuffix)
		newTagName := sb.String()

		// Git tag locally
		if err := gitTag(repo, newTagName); err != nil {
			if err == git.ErrTagExists {
				fmt.Fprintf(os.Stderr, "WARN: tag %s already exists in local! Skipping!\n", tag)
			} else {
				return err
			}
		}
		tagsToPush = append(tagsToPush, newTagName)
	}
	if config.DebugFlag {
		_, _ = fmt.Fprintf(os.Stdout, "LOG: Debug Flag detected. Skipping pushing of tags\n")
	} else {
		for _, tagToPush := range tagsToPush {
			if err := gitPushTag(repo, auth, tagToPush); err != nil {
				return err
			}
		}
	}
	return nil
}
