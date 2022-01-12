// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stdlib

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/testhelper"
	"golang.org/x/pkgsite/internal/version"
)

// A goRepo represents a git repo holding the Go standard library.
type goRepo interface {
	// Return the repo at the given version.
	repoAtVersion(version string) (*git.Repository, plumbing.ReferenceName, error)

	// Return all the refs of the repo.
	refs() ([]*plumbing.Reference, error)
}

type remoteGoRepo struct{}

// repoAtVersion returns a repo object for the Go repo at version by cloning the
// Go repo.
func (remoteGoRepo) repoAtVersion(v string) (_ *git.Repository, ref plumbing.ReferenceName, err error) {
	defer derrors.Wrap(&err, "remoteGoRepo.repoAtVersion(%q)", v)

	ref, err = refNameForVersion(v)
	if err != nil {
		return nil, "", err
	}
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           GoRepoURL,
		ReferenceName: ref,
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	})
	if err != nil {
		return nil, "", err
	}
	return repo, ref, nil
}

func (remoteGoRepo) refs() (_ []*plumbing.Reference, err error) {
	defer derrors.Wrap(&err, "remoteGoRepo.refs")

	re := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{GoRepoURL},
	})
	return re.List(&git.ListOptions{})
}

type localGoRepo struct {
	path string
	repo *git.Repository
}

func newLocalGoRepo(path string) (*localGoRepo, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	return &localGoRepo{
		path: path,
		repo: repo,
	}, nil
}

func (g *localGoRepo) repoAtVersion(v string) (_ *git.Repository, ref plumbing.ReferenceName, err error) {
	defer derrors.Wrap(&err, "localGoRepo(%s).repoAtVersion(%q)", g.path, v)
	ref, err = refNameForVersion(v)
	if err != nil {
		return nil, "", err
	}
	return g.repo, ref, nil
}

func (g *localGoRepo) refs() (rs []*plumbing.Reference, err error) {
	defer derrors.Wrap(&err, "localGoRepo(%s).refs", g.path)

	iter, err := g.repo.References()
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	err = iter.ForEach(func(r *plumbing.Reference) error {
		rs = append(rs, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rs, nil
}

type testGoRepo struct {
}

// repoAtVersion gets a Go repo for testing.
func (t *testGoRepo) repoAtVersion(v string) (_ *git.Repository, ref plumbing.ReferenceName, err error) {
	defer derrors.Wrap(&err, "testGoRepo.repoAtVersion(%q)", v)
	if v == TestMasterVersion {
		v = version.Master
	}
	if v == TestDevFuzzVersion {
		v = DevFuzz
	}
	fs := osfs.New(filepath.Join(testhelper.TestDataPath("testdata"), v))
	repo, err := git.Init(memory.NewStorage(), fs)
	if err != nil {
		return nil, "", fmt.Errorf("git.Init: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, "", fmt.Errorf("repo.Worktree: %v", err)
	}
	// Add all files in the directory.
	if _, err := wt.Add(""); err != nil {
		return nil, "", fmt.Errorf("wt.Add(): %v", err)
	}
	_, err = wt.Commit("", &git.CommitOptions{All: true, Author: &object.Signature{
		Name:  "Joe Random",
		Email: "joe@example.com",
		When:  TestCommitTime,
	}})
	if err != nil {
		return nil, "", fmt.Errorf("wt.Commit: %v", err)
	}
	return repo, plumbing.HEAD, nil
}

// References used for Versions during testing.
var testRefs = []plumbing.ReferenceName{
	// stdlib versions
	"refs/tags/go1.2.1",
	"refs/tags/go1.3.2",
	"refs/tags/go1.4.2",
	"refs/tags/go1.4.3",
	"refs/tags/go1.6",
	"refs/tags/go1.6.3",
	"refs/tags/go1.6beta1",
	"refs/tags/go1.8",
	"refs/tags/go1.8rc2",
	"refs/tags/go1.9rc1",
	"refs/tags/go1.11",
	"refs/tags/go1.12",
	"refs/tags/go1.12.1",
	"refs/tags/go1.12.5",
	"refs/tags/go1.12.9",
	"refs/tags/go1.13",
	"refs/tags/go1.13beta1",
	"refs/tags/go1.14.6",
	"refs/heads/dev.fuzz",
	"refs/heads/master",
	// other tags
	"refs/changes/56/93156/13",
	"refs/tags/release.r59",
	"refs/tags/weekly.2011-04-13",
}

func (t *testGoRepo) refs() ([]*plumbing.Reference, error) {
	var rs []*plumbing.Reference
	for _, r := range testRefs {
		// Only the name is ever used, so the referent can be empty.
		rs = append(rs, plumbing.NewSymbolicReference(r, ""))
	}
	return rs, nil
}
