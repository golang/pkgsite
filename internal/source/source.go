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
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// Info holds source information about a module, used to generate URLs referring
// to directories, files and lines.
type Info struct {
	repoURL   string       // URL of repo containing module; exported for DB schema compatibility
	moduleDir string       // directory of module relative to repo root
	commit    string       // tag or ID of commit corresponding to version
	templates urlTemplates // for building URLs
}

// RepoURL returns a URL for the home page of the repository.
func (i *Info) RepoURL() string {
	if i == nil {
		return ""
	}
	if i.templates.Repo == "" {
		// The default repo template is just "{repo}".
		return i.repoURL
	}
	return expand(i.templates.Repo, map[string]string{
		"repo": i.repoURL,
	})
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
		"repo":       i.repoURL,
		"importPath": path.Join(strings.TrimPrefix(i.repoURL, "https://"), dir),
		"commit":     i.commit,
		"dir":        path.Join(i.moduleDir, dir),
	}), "/")
}

// FileURL returns a URL for a file whose pathname is relative to the module's home directory.
func (i *Info) FileURL(pathname string) string {
	if i == nil {
		return ""
	}
	dir, base := path.Split(pathname)
	return expand(i.templates.File, map[string]string{
		"repo":       i.repoURL,
		"importPath": path.Join(strings.TrimPrefix(i.repoURL, "https://"), dir),
		"commit":     i.commit,
		"file":       path.Join(i.moduleDir, pathname),
		"base":       base,
	})
}

// LineURL returns a URL referring to a line in a file relative to the module's home directory.
func (i *Info) LineURL(pathname string, line int) string {
	if i == nil {
		return ""
	}
	dir, base := path.Split(pathname)
	return expand(i.templates.Line, map[string]string{
		"repo":       i.repoURL,
		"importPath": path.Join(strings.TrimPrefix(i.repoURL, "https://"), dir),
		"commit":     i.commit,
		"file":       path.Join(i.moduleDir, pathname),
		"base":       base,
		"line":       strconv.Itoa(line),
	})
}

// RawURL returns a URL referring to the raw contents of a file relative to the
// module's home directory.
func (i *Info) RawURL(pathname string) string {
	if i == nil {
		return ""
	}
	// Some templates don't support raw content serving.
	if i.templates.Raw == "" {
		return ""
	}
	moduleDir := i.moduleDir
	// Special case: the standard library's source module path is set to "src",
	// which is correct for source file links. But the README is at the repo
	// root, not in the src directory. In other words,
	// VersionInfo.LegacyReadmeFilePath is not relative to
	// VersionInfo.SourceInfo.moduleDir, as it is for every other module.
	// Correct for that here.
	if i.repoURL == stdlib.GoSourceRepoURL {
		moduleDir = ""
	}
	return expand(i.templates.Raw, map[string]string{
		"repo":   i.repoURL,
		"commit": i.commit,
		"file":   path.Join(moduleDir, pathname),
	})
}

// map of common urlTemplates
var urlTemplatesByKind = map[string]urlTemplates{
	"github":    githubURLTemplates,
	"gitlab":    githubURLTemplates, // preserved for backwards compatibility (DB still has source_info->Kind = "gitlab")
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
	// We used to use different templates for GitHub and GitLab. Now that
	// they're the same, prefer "github" for consistency (map random iteration
	// order means we could get either here).
	if ji.Kind == "gitlab" {
		ji.Kind = "github"
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

type Client struct {
	// client used for HTTP requests. It is mutable for testing purposes.
	httpClient *http.Client
}

// New constructs a *Client using the provided timeout.
func NewClient(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &ochttp.Transport{},
			Timeout:   timeout,
		},
	}
}

// doURL makes an HTTP request using the given url and method. It returns an
// error if the request returns an error. If only200 is true, it also returns an
// error if any status code other than 200 is returned.
func (c *Client) doURL(ctx context.Context, method, url string, only200 bool) (_ *http.Response, err error) {
	defer derrors.Wrap(&err, "doURL(ctx, client, %q, %q)", method, url)

	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("c.httpClient cannot be nil")
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := ctxhttp.Do(ctx, c.httpClient, req)
	if err != nil {
		return nil, err
	}
	if only200 && resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return resp, nil
}

// LegacyModuleInfo determines the repository corresponding to the module path. It
// returns a URL to that repo, as well as the directory of the module relative
// to the repo root.
//
// LegacyModuleInfo may fetch from arbitrary URLs, so it can be slow.
func ModuleInfo(ctx context.Context, client *Client, modulePath, version string) (info *Info, err error) {
	defer derrors.Wrap(&err, "source.LegacyModuleInfo(ctx, %q, %q)", modulePath, version)
	ctx, span := trace.StartSpan(ctx, "source.LegacyModuleInfo")
	defer span.End()

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
	repo, relativeModulePath, templates, transformCommit, err := matchStatic(modulePath)
	if err != nil {
		info, err = moduleInfoDynamic(ctx, client, modulePath, version)
		if err != nil {
			return nil, err
		}
	} else {
		commit, isHash := commitFromVersion(version, relativeModulePath)
		if transformCommit != nil {
			commit = transformCommit(commit, isHash)
		}
		info = &Info{
			repoURL:   "https://" + repo,
			moduleDir: relativeModulePath,
			commit:    commit,
			templates: templates,
		}
	}
	adjustVersionedModuleDirectory(ctx, client, info)
	return info, nil
	// TODO(golang/go#39627): support launchpad.net, including the special case
	// in cmd/go/internal/get/vcs.go.
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
func matchStatic(moduleOrRepoPath string) (repo, relativeModulePath string, _ urlTemplates, transformCommit func(string, bool) string, _ error) {
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
		return repo, relativeModulePath, pat.templates, pat.transformCommit, nil
	}
	return "", "", urlTemplates{}, nil, derrors.NotFound
}

// moduleInfoDynamic uses the go-import and go-source meta tags to construct an Info.
func moduleInfoDynamic(ctx context.Context, client *Client, modulePath, version string) (_ *Info, err error) {
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
	repoURL := sourceMeta.repoURL
	_, _, templates, transformCommit, _ := matchStatic(removeHTTPScheme(repoURL))
	// If err != nil, templates will be the zero value, so we can ignore it (same just below).
	if templates == (urlTemplates{}) {
		var repo string
		repo, _, templates, transformCommit, _ = matchStatic(removeHTTPScheme(sourceMeta.dirTemplate))
		if templates == (urlTemplates{}) {
			log.Infof(ctx, "no templates for repo URL %q from meta tag: err=%v", sourceMeta.repoURL, err)
		} else {
			// Use the repo from the template, not the original one.
			repoURL = "https://" + repo
		}
	}
	dir := strings.TrimPrefix(strings.TrimPrefix(modulePath, sourceMeta.repoRootPrefix), "/")
	commit, isHash := commitFromVersion(version, dir)
	if transformCommit != nil {
		commit = transformCommit(commit, isHash)
	}
	return &Info{
		repoURL:   strings.TrimSuffix(repoURL, "/"),
		moduleDir: dir,
		commit:    commit,
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
func adjustVersionedModuleDirectory(ctx context.Context, client *Client, info *Info) {
	dirWithoutVersion := removeVersionSuffix(info.moduleDir)
	if info.moduleDir == dirWithoutVersion {
		return
	}
	// moduleDir does have a "/vN" for N > 1. To see if that is the actual directory,
	// fetch the go.mod file from it.
	res, err := client.doURL(ctx, "HEAD", info.FileURL("go.mod"), true)
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
	pattern   string // uncompiled regexp
	templates urlTemplates
	re        *regexp.Regexp
	// transformCommit may alter the commit before substitution
	transformCommit func(commit string, isHash bool) string
}{
	{
		pattern:   `^(?P<repo>github\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`,
		templates: githubURLTemplates,
	},
	{
		pattern:   `^(?P<repo>bitbucket\.org/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`,
		templates: bitbucketURLTemplates,
	},
	{
		pattern:   `^(?P<repo>gitlab\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`,
		templates: githubURLTemplates,
	},
	{
		// Assume that any site beginning with "gitlab." works like gitlab.com.
		pattern:   `^(?P<repo>gitlab\.[a-z0-9A-Z.-]+/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		templates: githubURLTemplates,
	},
	{
		pattern:   `^(?P<repo>gitee\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		templates: githubURLTemplates,
	},
	{
		pattern: `^(?P<repo>git\.sr\.ht/~[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`,
		templates: urlTemplates{
			Directory: "{repo}/tree/{commit}/{dir}",
			File:      "{repo}/tree/{commit}/{file}",
			Line:      "{repo}/tree/{commit}/{file}#L{line}",
			Raw:       "{repo}/blob/{commit}/{file}",
		},
	},
	{
		pattern: `^(?P<repo>git\.fd\.io/[a-z0-9A-Z_.\-]+)`,
		templates: urlTemplates{
			Directory: "{repo}/tree/{dir}?{commit}",
			File:      "{repo}/tree/{file}?{commit}",
			Line:      "{repo}/tree/{file}?{commit}#n{line}",
			Raw:       "{repo}/plain/{file}?{commit}",
		},
		transformCommit: func(commit string, isHash bool) string {
			// hashes use "?id=", tags use "?h="
			p := "h"
			if isHash {
				p = "id"
			}
			return fmt.Sprintf("%s=%s", p, commit)
		},
	},
	{
		pattern: `^(?P<repo>git\.pirl\.io/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)`,
		templates: urlTemplates{
			Directory: "{repo}/-/tree/{commit}/{dir}",
			File:      "{repo}/-/blob/{commit}/{file}",
			Line:      "{repo}/-/blob/{commit}/{file}#L{line}",
			Raw:       "{repo}/-/raw/{commit}/{file}",
		},
	},
	{
		pattern:         `^(?P<repo>gitea\.com/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		templates:       giteaURLTemplates,
		transformCommit: giteaTransformCommit,
	},
	{
		// Assume that any site beginning with "gitea." works like gitea.com.
		pattern:         `^(?P<repo>gitea\.[a-z0-9A-Z.-]+/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		templates:       giteaURLTemplates,
		transformCommit: giteaTransformCommit,
	},
	{
		pattern:         `^(?P<repo>go\.isomorphicgo\.org/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		templates:       giteaURLTemplates,
		transformCommit: giteaTransformCommit,
	},
	{
		pattern:         `^(?P<repo>git\.openprivacy\.ca/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		templates:       giteaURLTemplates,
		transformCommit: giteaTransformCommit,
	},
	{
		pattern: `^(?P<repo>gogs\.[a-z0-9A-Z.-]+/[a-z0-9A-Z_.\-]+/[a-z0-9A-Z_.\-]+)(\.git|$)`,
		// Gogs uses the same basic structure as Gitea, but omits the type of
		// commit ("tag" or "commit"), so we don't need a transformCommit
		// function. Gogs does not support short hashes, but we create those
		// URLs anyway. See gogs/gogs#6242.
		templates: giteaURLTemplates,
	},

	{
		pattern: `^(?P<repo>dmitri\.shuralyov\.com\/.+)$`,
		templates: urlTemplates{
			Repo:      "{repo}/...",
			Directory: "https://gotools.org/{importPath}?rev={commit}",
			File:      "https://gotools.org/{importPath}?rev={commit}#{base}",
			Line:      "https://gotools.org/{importPath}?rev={commit}#{base}-L{line}",
		},
	},

	// Patterns that match the general go command pattern, where they must have
	// a ".git" repo suffix in an import path. If matching a repo URL from a meta tag,
	// there is no ".git".
	{
		pattern: `^(?P<repo>[^.]+\.googlesource\.com/[^.]+)(\.git|$)`,
		templates: urlTemplates{
			Directory: "{repo}/+/{commit}/{dir}",
			File:      "{repo}/+/{commit}/{file}",
			Line:      "{repo}/+/{commit}/{file}#{line}",
			// Gitiles has no support for serving raw content at this time.
		},
	},
	{
		pattern:   `^(?P<repo>git\.apache\.org/[^.]+)(\.git|$)`,
		templates: githubURLTemplates,
	},
	// General syntax for the go command. We can extract the repo and directory, but
	// we don't know the URL templates.
	// Must be last in this list.
	{
		pattern:   `(?P<repo>([a-z0-9.\-]+\.)+[a-z0-9.\-]+(:[0-9]+)?(/~?[A-Za-z0-9_.\-]+)+?)\.(bzr|fossil|git|hg|svn)`,
		templates: urlTemplates{},
	},
}

func init() {
	for i := range patterns {
		re := regexp.MustCompile(patterns[i].pattern)
		// The pattern regexp must contain a group named "repo".
		found := false
		for _, n := range re.SubexpNames() {
			if n == "repo" {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("pattern %s missing <repo> group", patterns[i].pattern))
		}
		patterns[i].re = re
	}
}

// giteaTransformCommit transforms commits for the Gitea code hosting system.
func giteaTransformCommit(commit string, isHash bool) string {
	// Hashes use "commit", tags use "tag".
	// Short hashes aren't currently supported, but we build the URL
	// anyway in the hope that someday they will be.
	if isHash {
		return "commit/" + commit
	}
	return "tag/" + commit
}

// urlTemplates describes how to build URLs from bits of source information.
// The fields are exported for JSON encoding.
//
// The template variables are:
//
// 	• {repo}       - Repository URL with "https://" prefix ("https://example.com/myrepo").
// 	• {importPath} - Package import path ("example.com/myrepo/mypkg").
// 	• {commit}     - Tag name or commit hash corresponding to version ("v0.1.0" or "1234567890ab").
// 	• {dir}        - Path to directory of the package, relative to repo root ("mypkg").
// 	• {file}       - Path to file containing the identifier, relative to repo root ("mypkg/file.go").
// 	• {base}       - Base name of file containing the identifier, including file extension ("file.go").
// 	• {line}       - Line number for the identifier ("41").
//
type urlTemplates struct {
	Repo      string `json:",omitempty"` // Optional URL template for the repository home page, with {repo}. If left empty, a default template "{repo}" is used.
	Directory string // URL template for a directory, with {repo}, {importPath}, {commit}, {dir}.
	File      string // URL template for a file, with {repo}, {importPath}, {commit}, {file}, {base}.
	Line      string // URL template for a line, with {repo}, {importPath}, {commit}, {file}, {base}, {line}.
	Raw       string // Optional URL template for the raw contents of a file, with {repo}, {commit}, {file}.
}

var (
	githubURLTemplates = urlTemplates{
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
	giteaURLTemplates = urlTemplates{
		Directory: "{repo}/src/{commit}/{dir}",
		File:      "{repo}/src/{commit}/{file}",
		Line:      "{repo}/src/{commit}/{file}#L{line}",
		Raw:       "{repo}/raw/{commit}/{file}",
	}
)

// commitFromVersion returns a string that refers to a commit corresponding to version.
// It also reports whether the returned value is a commit hash.
// The string may be a tag, or it may be the hash or similar unique identifier of a commit.
// The second argument is the module path relative to the repo root.
func commitFromVersion(vers, relativeModulePath string) (commit string, isHash bool) {
	// Commit for the module: either a sha for pseudoversions, or a tag.
	v := strings.TrimSuffix(vers, "+incompatible")
	if version.IsPseudo(v) {
		// Use the commit hash at the end.
		return v[strings.LastIndex(v, "-")+1:], true
	} else {
		// The tags for a nested module begin with the relative module path of the module,
		// removing a "/vN" suffix if N > 1.
		prefix := removeVersionSuffix(relativeModulePath)
		if prefix != "" {
			return prefix + "/" + v, false
		}
		return v, false
	}
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
