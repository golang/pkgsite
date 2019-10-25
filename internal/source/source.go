// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package source constructs public URLs that link to the source files in a module. It
// can be used to build references to Go source code, or to any other files in a
// module.
//
// Of course, the module zip file contains all the files in the module. This
// package attempts to find the origin of the zip file, in a repository that is
// publicly readable, and constructs links to that repo. While a module zip file
// could in theory come from anywhere, including a non-public location, this
// package recognizes standard module path patterns and construct repository
// URLs from them, like the go command does.
package source

//
// Much of this code was adapted from
// https://go.googlesource.com/gddo/+/refs/heads/master/gosrc
// and
// https://go.googlesource.com/go/+/refs/heads/master/src/cmd/go/internal/get

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/version"
	"golang.org/x/net/context/ctxhttp"
)

// Info holds source information about a module, used to generate URLs referring
// to directories, files and lines.
type Info struct {
	// TODO(b/141771951): change the DB schema of versions to include this information
	repoURL   string       // URL of repo containing module; exported for DB schema compatibility
	moduleDir string       // directory of module relative to repo root
	commit    string       // tag or ID of commit corresponding to version
	templates urlTemplates // for building URLs
}

func (i *Info) RepoURL() string {
	if i == nil {
		return ""
	}
	return i.repoURL
}

// ModuleURL returns a URL for the home page of the module.
func (i *Info) ModuleURL() string {
	return i.DirectoryURL("")
}

// DirectoryURL returns a URL for a directory relative to the module's home directory.
func (i *Info) DirectoryURL(dir string) string {
	if i == nil {
		return ""
	}
	return strings.TrimSuffix(expand(i.templates.Directory, map[string]string{
		"repo":   i.repoURL,
		"commit": i.commit,
		"dir":    path.Join(i.moduleDir, dir),
	}), "/")
}

// FileURL returns a URL for a file whose pathname is relative to the module's home directory.
func (i *Info) FileURL(pathname string) string {
	if i == nil {
		return ""
	}
	return expand(i.templates.File, map[string]string{
		"repo":   i.repoURL,
		"commit": i.commit,
		"file":   path.Join(i.moduleDir, pathname),
	})
}

// LineURL returns a URL referring to a line in a file relative to the module's home directory.
func (i *Info) LineURL(pathname string, line int) string {
	if i == nil {
		return ""
	}
	return expand(i.templates.Line, map[string]string{
		"repo":   i.repoURL,
		"commit": i.commit,
		"file":   path.Join(i.moduleDir, pathname),
		"line":   strconv.Itoa(line),
	})
}

// RawURL returns a URL referring to the raw contents of a file relative to the
// module's home directory. In addition to the usual variables, it supports
// {repoPath}, which is the repo URL's path.
func (i *Info) RawURL(pathname string) string {
	if i == nil {
		return ""
	}
	// Some templates don't support raw content serving.
	if i.templates.Raw == "" {
		return ""
	}
	u, err := url.Parse(i.repoURL)
	if err != nil {
		// This should never happen. If it does, note it and soldier on.
		log.Errorf("repo URL %q failed to parse: %v", i.repoURL, err)
		u = &url.URL{Path: "ERROR"}
	}

	moduleDir := i.moduleDir
	// Special case: the standard library's source module path is set to "src",
	// which is correct for source file links. But the README is at the repo
	// root, not in the src directory. In other words,
	// VersionInfo.ReadmeFilePath is not relative to
	// VersionInfo.SourceInfo.moduleDir, as it is for every other module.
	// Correct for that here.
	if i.repoURL == stdlib.GoSourceRepoURL {
		moduleDir = ""
	}
	return expand(i.templates.Raw, map[string]string{
		"repo":     i.repoURL,
		"repoPath": strings.TrimPrefix(u.Path, "/"),
		"commit":   i.commit,
		"file":     path.Join(moduleDir, pathname),
	})
}

// map of common urlTemplates
var urlTemplatesByKind = map[string]urlTemplates{
	"github":    githubURLTemplates,
	"gitlab":    gitlabURLTemplates,
	"bitbucket": bitbucketURLTemplates,
}

// jsonInfo is a Go struct describing the JSON structure of an INFO.
type jsonInfo struct {
	RepoURL   string
	ModuleDir string
	Commit    string
	// Store common templates efficiently by setting this to a short string
	// we look up in a map. If Kind != "", then Templates == nil.
	Kind      string        `json:",omitempty"`
	Templates *urlTemplates `json:",omitempty"`
}

// ToJSONForDB returns the Info encoded for storage in the database.
func (i *Info) MarshalJSON() (_ []byte, err error) {
	defer derrors.Wrap(&err, "MarshalJSON")

	ji := &jsonInfo{
		RepoURL:   i.repoURL,
		ModuleDir: i.moduleDir,
		Commit:    i.commit,
	}
	// Store common templates efficiently, by name.
	for kind, templs := range urlTemplatesByKind {
		if i.templates == templs {
			ji.Kind = kind
			break
		}
	}
	if ji.Kind == "" && i.templates != (urlTemplates{}) {
		ji.Templates = &i.templates
	}
	return json.Marshal(ji)
}

func (i *Info) UnmarshalJSON(data []byte) (err error) {
	defer derrors.Wrap(&err, "UnmarshalJSON(data)")

	var ji jsonInfo
	if err := json.Unmarshal(data, &ji); err != nil {
		return err
	}
	i.repoURL = ji.RepoURL
	i.moduleDir = ji.ModuleDir
	i.commit = ji.Commit
	if ji.Kind != "" {
		i.templates = urlTemplatesByKind[ji.Kind]
	} else if ji.Templates != nil {
		i.templates = *ji.Templates
	}
	return nil
}

// ModuleInfo determines the repository corresponding to the module path. It
// returns a URL to that repo, as well as the directory of the module relative
// to the repo root.
//
// ModuleInfo may fetch from arbitrary URLs, so it can be slow.
func ModuleInfo(ctx context.Context, client *http.Client, modulePath, version string) (info *Info, err error) {
	defer derrors.Wrap(&err, "source.ModuleInfo(ctx, %q, %q)", modulePath, version)

	if modulePath == stdlib.ModulePath {
		commit, err := stdlib.TagForVersion(version)
		if err != nil {
			return nil, err
		}
		return &Info{
			repoURL:   stdlib.GoSourceRepoURL,
			moduleDir: stdlib.Directory(version),
			commit:    commit,
			templates: githubURLTemplates,
		}, nil
	}
	// Don't let requests to arbitrary URLs take too long.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	repo, relativeModulePath, templates, err := matchStatic(modulePath)
	if err != nil {
		info, err = moduleInfoDynamic(ctx, client, modulePath, version)
		if err != nil {
			return nil, err
		}
	} else {
		info = &Info{
			repoURL:   "https://" + repo,
			moduleDir: relativeModulePath,
			commit:    commitFromVersion(version, relativeModulePath),
			templates: templates,
		}
	}
	adjustVersionedModuleDirectory(ctx, client, info)
	return info, nil
	// TODO(b/141770842): support launchpad.net, including the special case in cmd/go/internal/get/vcs.go.
}

// matchStatic matches the given module or repo path against a list of known
// patterns. It returns the repo name, the module path relative to the repo
// root, and URL templates if there is a match.
//
// The relative module path may not be correct in all cases: it is wrong if it
// ends in a version that is not part of the repo directory structure, because
// the repo follows the "major branch" convention for versions 2 and above.
// E.g. this function could return "foo/v2", but the module files live under "foo"; the
// "/v2" is part of the module path (and the import paths of its packages) but
// is not a subdirectory. This mistake is corrected in adjustVersionedModuleDirectory,
// once we have all the information we need to fix it.
//
// repo + "/" + relativeModulePath is often, but not always, equal to
// moduleOrRepoPath. It is not when the argument is a module path that uses the
// go command's general syntax, which ends in a ".vcs" (e.g. ".git", ".hg") that
// is neither part of the repo nor the suffix. For example, if the argument is
//   github.com/a/b/c
// then repo="github.com/a/b" and relativeModulePath="c"; together they make up the module path.
// But if the argument is
//   example.com/a/b.git/c
// then repo="example.com/a/b" and relativeModulePath="c"; the ".git" is omitted, since it is neither
// part of the repo nor part of the relative path to the module within the repo.
func matchStatic(moduleOrRepoPath string) (repo, relativeModulePath string, _ urlTemplates, _ error) {
	for _, pat := range patterns {
		matches := pat.re.FindStringSubmatch(moduleOrRepoPath)
		if matches == nil {
			continue
		}
		var repo string
		for i, n := range pat.re.SubexpNames() {
			if n == "repo" {
				repo = matches[i]
				break
			}
		}
		// Special case: git.apache.org has a go-import tag that points to
		// github.com/apache, but it's not quite right (the repo prefix is
		// missing a ".git"), so handle it here.
		const apacheDomain = "git.apache.org/"
		if strings.HasPrefix(repo, apacheDomain) {
			repo = strings.Replace(repo, apacheDomain, "github.com/apache/", 1)
		}
		relativeModulePath = strings.TrimPrefix(moduleOrRepoPath, matches[0])
		relativeModulePath = strings.TrimPrefix(relativeModulePath, "/")
		return repo, relativeModulePath, pat.templates, nil
	}
	return "", "", urlTemplates{}, derrors.NotFound
}

// moduleInfoDynamic uses the go-import and go-source meta tags to construct an Info.
func moduleInfoDynamic(ctx context.Context, client *http.Client, modulePath, version string) (_ *Info, err error) {
	defer derrors.Wrap(&err, "source.moduleInfoDynamic(ctx, client, %q, %q)", modulePath, version)

	sourceMeta, err := fetchMeta(ctx, client, modulePath)
	if err != nil {
		return nil, err
	}
	// Don't check that the tag information at the repo root prefix is the same
	// as in the module path. It was done for us by the proxy and/or go command.
	// (This lets us merge information from the go-import and go-source tags.)

	// sourceMeta contains some information about where the module's source lives. But there
	// are some problems:
	// - We may only have a go-import tag, not a go-source tag, so we don't have URL templates for
	//   building URLs to files and directories.
	// - Even if we do have a go-source tag, its URL template format predates
	//   versioning, so the URL templates won't provide a way to specify a
	//   version or commit.
	//
	// We resolve these problems as follows:
	// 1. First look at the repo URL from the tag. If that matches a known hosting site, use the
	//    URL templates corresponding to that site and ignore whatever's in the tag.
	// 2. Then look at the URL templates to see if they match a known pattern, and use the templates
	//    from that pattern. For example, the meta tags for gopkg.in/yaml.v2 only mention github
	//    in the URL templates, like "https://github.com/go-yaml/yaml/tree/v2.2.3{/dir}". We can observe
	//    that that template begins with a known pattern--a GitHub repo, ignore the rest of it, and use the
	//    GitHub URL templates that we know.
	// 3. TODO(b/141847689): heuristically determine how to construct a URL template with a commit from the
	//    existing go-source template. For example, by replacing "master" with "{commit}".
	// We could also consider using the repo in the go-import tag instead of the one in the go-source tag,
	// if the former matches a known pattern but the latter does not.
	repoURL := sourceMeta.repoURL
	_, _, templates, _ := matchStatic(removeHTTPScheme(repoURL))
	// If err != nil, templates will the zero value, so we can ignore it (same just below).
	if templates == (urlTemplates{}) {
		var repo string
		repo, _, templates, _ = matchStatic(removeHTTPScheme(sourceMeta.dirTemplate))
		if templates == (urlTemplates{}) {
			log.Infof("no templates for repo URL %q from meta tag: err=%v", sourceMeta.repoURL, err)
		} else {
			// Use the repo from the template, not the original one.
			repoURL = "https://" + repo
		}
	}
	dir := strings.TrimPrefix(strings.TrimPrefix(modulePath, sourceMeta.repoRootPrefix), "/")
	return &Info{
		repoURL:   strings.TrimSuffix(repoURL, "/"),
		moduleDir: dir,
		commit:    commitFromVersion(version, dir),
		templates: templates,
	}, nil
}

// adjustVersionedModuleDirectory changes info.moduleDir if necessary to
// correctly reflect the repo structure. info.moduleDir will be wrong if it has
// a suffix "/vN" for N > 1, and the repo uses the "major branch" convention,
// where modules at version 2 and higher live on branches rather than
// subdirectories. See https://research.swtch.com/vgo-module for a discussion of
// the "major branch" vs. "major subdirectory" conventions for organizing a
// repo.
func adjustVersionedModuleDirectory(ctx context.Context, client *http.Client, info *Info) {
	dirWithoutVersion := removeVersionSuffix(info.moduleDir)
	if info.moduleDir == dirWithoutVersion {
		return
	}
	// moduleDir does have a "/vN" for N > 1. To see if that is the actual directory,
	// fetch the go.mod file from it.
	res, err := doURL(ctx, client, "HEAD", info.FileURL("go.mod"))
	// On any failure, assume that the right directory is the one without the version.
	if err != nil {
		info.moduleDir = dirWithoutVersion
	} else {
		res.Body.Close()
	}
}

// removeHTTPScheme removes an initial "http://" or "https://" from url.
// The result can be used to match against our static patterns.
// If the URL uses a different scheme, it won't be removed and it won't
// match any patterns, as intended.
func removeHTTPScheme(url string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, prefix) {
			return url[len(prefix):]
		}
	}
	return url
}

// removeVersionSuffix returns s with "/vN" removed if N is an integer > 1.
// Otherwise it returns s.
func removeVersionSuffix(s string) string {
	dir, base := path.Split(s)
	if !strings.HasPrefix(base, "v") {
		return s
	}
	if n, err := strconv.Atoi(base[1:]); err != nil || n < 2 {
		return s
	}
	return strings.TrimSuffix(dir, "/")
}

// Patterns for determining repo and URL templates from module paths or repo
// URLs. Each regexp must match a prefix of the target string, and must have a
// group named "repo".
var patterns = []struct {
	re        *regexp.Regexp
	templates urlTemplates
}{
	// Patterns known to the go command.
	{
		regexp.MustCompile(`^(?P<repo>github\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`),
		githubURLTemplates,
	},
	{
		regexp.MustCompile(`^(?P<repo>bitbucket\.org/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`),
		bitbucketURLTemplates,
	},
	// Other patterns from cmd/go/internal/get/vcs.go, that we omit:
	// hub.jazz.net it no longer exists.
	// git.apache.org now redirects to github, and serves a go-import tag.
	// git.openstack.org has been rebranded.
	// chiselapp.com has no Go packages in godoc.org.

	// Patterns that are not (yet) part of the go command.
	{
		regexp.MustCompile(`^(?P<repo>gitlab\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`),
		gitlabURLTemplates,
	},
	{
		// Assume that any site beginning "gitlab." works like gitlab.com.
		regexp.MustCompile(`^(?P<repo>gitlab\.[a-z0-9A-Z.-]+/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`),
		gitlabURLTemplates,
	},
	{
		regexp.MustCompile(`^(?P<repo>gitee\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`),
		gitlabURLTemplates,
	},

	// Patterns that match the general go command pattern, where they must have
	// a ".git" repo suffix in an import path. If matching a repo URL from a meta tag,
	// there is no ".git".
	{
		regexp.MustCompile(`^(?P<repo>[^.]+\.googlesource\.com/[^.]+)(\.git|$)`),
		urlTemplates{
			Directory: "{repo}/+/{commit}/{dir}",
			File:      "{repo}/+/{commit}/{file}",
			Line:      "{repo}/+/{commit}/{file}#{line}",
			// no raw support (b/13912564)
		},
	},
	{
		regexp.MustCompile(`^(?P<repo>git\.apache\.org/[^.]+)(\.git|$)`),
		githubURLTemplates,
	},
	// General syntax for the go command. We can extract the repo and directory, but
	// we don't know the URL templates.
	// Must be last in this list.
	{
		regexp.MustCompile(`(?P<repo>([a-z0-9.\-]+\.)+[a-z0-9.\-]+(:[0-9]+)?(/~?[A-Za-z0-9_.\-]+)+?)\.(bzr|fossil|git|hg|svn)`),
		urlTemplates{},
	},
}

func init() {
	for _, p := range patterns {
		found := false
		for _, n := range p.re.SubexpNames() {
			if n == "repo" {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("pattern %s missing <repo> group", p.re))
		}
	}
}

// urlTemplates describes how to build URLs from bits of source information.
// The fields are exported for JSON encoding.
type urlTemplates struct {
	Directory string // URL template for a directory, with {repo}, {commit} and {dir}
	File      string // URL template for a file, with {repo}, {commit} and {file}
	Line      string // URL template for a line, with {repo}, {commit}, {file} and {line}
	Raw       string // URL template for the raw contents of a file, with {repo}, {repoPath}, {commit} and {file}
}

var (
	githubURLTemplates = urlTemplates{
		Directory: "{repo}/tree/{commit}/{dir}",
		File:      "{repo}/blob/{commit}/{file}",
		Line:      "{repo}/blob/{commit}/{file}#L{line}",
		Raw:       "https://raw.githubusercontent.com/{repoPath}/{commit}/{file}",
	}

	gitlabURLTemplates = urlTemplates{
		Directory: "{repo}/tree/{commit}/{dir}",
		File:      "{repo}/blob/{commit}/{file}",
		Line:      "{repo}/blob/{commit}/{file}#L{line}",
		Raw:       "{repo}/raw/{commit}/{file}",
	}

	bitbucketURLTemplates = urlTemplates{
		Directory: "{repo}/src/{commit}/{dir}",
		File:      "{repo}/src/{commit}/{file}",
		Line:      "{repo}/src/{commit}/{file}#lines-{line}",
		Raw:       "{repo}/raw/{commit}/{file}",
	}
)

// commitFromVersion returns a string that refers to a commit corresponding to version.
// The string may be a tag, or it may be the hash or similar unique identifier of a commit.
// The second argument is the module path relative to the repo root.
func commitFromVersion(vers, relativeModulePath string) string {
	// Commit for the module: either a sha for pseudoversions, or a tag.
	v := strings.TrimSuffix(vers, "+incompatible")
	if version.IsPseudo(v) {
		// Use the commit hash at the end.
		return v[strings.LastIndex(v, "-")+1:]
	} else {
		// The tags for a nested module begin with the relative module path of the module,
		// removing a "/vN" suffix if N > 1.
		prefix := removeVersionSuffix(relativeModulePath)
		if prefix != "" {
			return prefix + "/" + v
		}
		return v
	}
}

// doURL makes an HTTP request using the given url and method. It returns an
// error if the request returns an error or if any status code other than 200 is
// returned.
func doURL(ctx context.Context, client *http.Client, method, url string) (_ *http.Response, err error) {
	defer derrors.Wrap(&err, "doURL(ctx, client, %q, %q)", method, url)

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return resp, nil
}

// The following code copied from cmd/go/internal/get:

// expand rewrites s to replace {k} with match[k] for each key k in match.
func expand(s string, match map[string]string) string {
	// We want to replace each match exactly once, and the result of expansion
	// must not depend on the iteration order through the map.
	// A strings.Replacer has exactly the properties we're looking for.
	oldNew := make([]string, 0, 2*len(match))
	for k, v := range match {
		oldNew = append(oldNew, "{"+k+"}", v)
	}
	return strings.NewReplacer(oldNew...).Replace(s)
}

// NewGitHubInfo creates a source.Info with GitHub URL templates.
// It is for testing only.
func NewGitHubInfo(repoURL, moduleDir, commit string) *Info {
	return &Info{
		repoURL:   repoURL,
		moduleDir: moduleDir,
		commit:    commit,
		templates: githubURLTemplates,
	}
}

// NewGitLabInfo creates a source.Info with GitHub URL templates.
// It is for testing only.
func NewGitLabInfo(repoURL, moduleDir, commit string) *Info {
	return &Info{
		repoURL:   repoURL,
		moduleDir: moduleDir,
		commit:    commit,
		templates: gitlabURLTemplates,
	}
}
