// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package derrors defines internal error values to categorize the different
// types error semantics we support.
package derrors

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
)

//lint:file-ignore ST1012 prefixing error values with Err would stutter

var (
	// Unsupported operation indicates that a requested operation cannot be performed, because it
	// is unsupported. It is used here instead of errors.ErrUnsupported until we are able to depend
	// on Go 1.21 in the pkgsite repo.
	Unsupported = errors.New("unsupported operation")

	// HasIncompletePackages indicates a module containing packages that
	// were processed with a 60x error code.
	HasIncompletePackages = errors.New("has incomplete packages")

	// NotFound indicates that a requested entity was not found (HTTP 404).
	NotFound = errors.New("not found")

	// NotFetched means that the proxy returned "not found" with the
	// Disable-Module-Fetch header set. We don't know if the module really
	// doesn't exist, or the proxy just didn't fetch it.
	NotFetched = errors.New("not fetched by proxy")

	// InvalidArgument indicates that the input into the request is invalid in
	// some way (HTTP 400).
	InvalidArgument = errors.New("invalid argument")
	// BadModule indicates a problem with a module.
	BadModule = errors.New("bad module")
	// Excluded indicates that the module is excluded. (See internal/postgres/excluded.go.)
	Excluded = errors.New("excluded")

	// AlternativeModule indicates that the path of the module zip file differs
	// from the path specified in the go.mod file.
	AlternativeModule = errors.New("alternative module")

	// ModuleTooLarge indicates that the module is too large for us to process.
	// This should be temporary: we should obtain sufficient resources to process
	// any module, up to the max size allowed by the proxy.
	ModuleTooLarge = errors.New("module too large")

	// SheddingLoad indicates that the server is overloaded and cannot process the
	// module at this time.
	SheddingLoad = errors.New("shedding load")

	// Cleaned indicates that the module version was cleaned from the DB and
	// shouldn't be reprocessed.
	Cleaned = errors.New("cleaned")

	// Unknown indicates that the error has unknown semantics.
	Unknown = errors.New("unknown")

	// ProxyTimedOut indicates that a request timed out when fetching from the Module Mirror.
	ProxyTimedOut = errors.New("proxy timed out")

	// ProxyError is used to capture non-actionable server errors returned from the proxy.
	ProxyError = errors.New("proxy error")

	// VulnDBError is used to capture non-actionable server errors returned from vulndb.
	VulnDBError = errors.New("vulndb error")

	// PackageBuildContextNotSupported indicates that the build context for the
	// package is not supported.
	PackageBuildContextNotSupported = errors.New("package build context not supported")
	// PackageMaxImportsLimitExceeded indicates that the package has too many
	// imports.
	PackageMaxImportsLimitExceeded = errors.New("package max imports limit exceeded")
	// PackageMaxFileSizeLimitExceeded indicates that the package contains a file
	// that exceeds fetch.MaxFileSize.
	PackageMaxFileSizeLimitExceeded = errors.New("package max file size limit exceeded")
	// PackageDocumentationHTMLTooLarge indicates that the rendered documentation
	// HTML size exceeded the specified limit for dochtml.RenderOptions.
	PackageDocumentationHTMLTooLarge = errors.New("package documentation HTML is too large")
	// PackageBadImportPath represents an error loading a package because its
	// contents do not make up a valid package. This can happen, for
	// example, if the .go files fail to parse or declare different package
	// names.
	// Go files were found in a directory, but the resulting import path is invalid.
	PackageBadImportPath = errors.New("package bad import path")
	// PackageInvalidContents represents an error loading a package because
	// its contents do not make up a valid package. This can happen, for
	// example, if the .go files fail to parse or declare different package
	// names.
	PackageInvalidContents = errors.New("package invalid contents")

	// DBModuleInsertInvalid represents a module that was successfully
	// fetched but could not be inserted due to invalid arguments to
	// postgres.InsertModule.
	DBModuleInsertInvalid = errors.New("db module insert invalid")

	// ReprocessStatusOK indicates that the module to be reprocessed
	// previously had a status of http.StatusOK.
	ReprocessStatusOK = errors.New("reprocess status ok")
	// ReprocessHasIncompletePackages indicates that the module to be reprocessed
	// previously had a status of 290.
	ReprocessHasIncompletePackages = errors.New("reprocess has incomplete packages")
	// ReprocessBadModule indicates that the module to be reprocessed
	// previously had a status of derrors.BadModule.
	ReprocessBadModule = errors.New("reprocess bad module")
	// ReprocessAlternativeModule indicates that the module to be reprocessed
	// previously had a status of derrors.AlternativeModule.
	ReprocessAlternative = errors.New("reprocess alternative module")
	// ReprocessDBModuleInsertInvalid represents a module to be reprocessed
	// that was successfully fetched but could not be inserted due to invalid
	// arguments to postgres.InsertModule.
	ReprocessDBModuleInsertInvalid = errors.New("reprocess db module insert invalid")
)

var codes = []struct {
	err  error
	code int
}{
	{NotFound, http.StatusNotFound},
	{InvalidArgument, http.StatusBadRequest},
	{Excluded, http.StatusForbidden},
	{SheddingLoad, http.StatusServiceUnavailable},

	// Since the following aren't HTTP statuses, pick unused codes.
	{HasIncompletePackages, 290},
	{DBModuleInsertInvalid, 480},
	{NotFetched, 481},
	{BadModule, 490},
	{AlternativeModule, 491},
	{ModuleTooLarge, 492},
	{Cleaned, 493},

	{ProxyTimedOut, 550}, // not a real code
	{ProxyError, 551},    // not a real code
	{VulnDBError, 552},   // not a real code
	// 52x and 54x errors represents modules that need to be reprocessed, and the
	// previous status code the module had. Note that the status code
	// matters for determining reprocessing order.
	{ReprocessStatusOK, 520},
	{ReprocessHasIncompletePackages, 521},
	{ReprocessBadModule, 540},
	{ReprocessAlternative, 541},
	{ReprocessDBModuleInsertInvalid, 542},

	// 60x errors represents errors that occurred when processing a
	// package.
	{PackageBuildContextNotSupported, 600},
	{PackageMaxImportsLimitExceeded, 601},
	{PackageMaxFileSizeLimitExceeded, 602},
	{PackageDocumentationHTMLTooLarge, 603},
	{PackageInvalidContents, 604},
	{PackageBadImportPath, 605},
}

// FromStatus generates an error according for the given status code. It uses
// the given format string and arguments to create the error string according
// to the fmt package. If format is the empty string, then the error
// corresponding to the code is returned unwrapped.
//
// If code is http.StatusOK, it returns nil.
func FromStatus(code int, format string, args ...any) error {
	if code == http.StatusOK {
		return nil
	}
	var innerErr = Unknown
	for _, e := range codes {
		if e.code == code {
			innerErr = e.err
			break
		}
	}
	if format == "" {
		return innerErr
	}
	return fmt.Errorf(format+": %w", append(args, innerErr)...)
}

// ToStatus returns a status code corresponding to err.
func ToStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	for _, e := range codes {
		if errors.Is(err, e.err) {
			return e.code
		}
	}
	return http.StatusInternalServerError
}

// ToReprocessStatus returns the reprocess status code corresponding to the
// provided status.
func ToReprocessStatus(status int) int {
	switch status {
	case http.StatusOK:
		return ToStatus(ReprocessStatusOK)
	case ToStatus(HasIncompletePackages):
		return ToStatus(ReprocessHasIncompletePackages)
	case ToStatus(BadModule):
		return ToStatus(ReprocessBadModule)
	case ToStatus(AlternativeModule):
		return ToStatus(ReprocessAlternative)
	case ToStatus(DBModuleInsertInvalid):
		return ToStatus(ReprocessDBModuleInsertInvalid)
	default:
		return status
	}
}

// Add adds context to the error.
// The result cannot be unwrapped to recover the original error.
// It does nothing when *errp == nil.
//
// Example:
//
//	defer derrors.Add(&err, "copy(%s, %s)", src, dst)
//
// See Wrap for an equivalent function that allows
// the result to be unwrapped.
func Add(errp *error, format string, args ...any) {
	if *errp != nil {
		*errp = fmt.Errorf("%s: %v", fmt.Sprintf(format, args...), *errp)
	}
}

// Wrap adds context to the error and allows
// unwrapping the result to recover the original error.
//
// Example:
//
//	defer derrors.Wrap(&err, "copy(%s, %s)", src, dst)
//
// See Add for an equivalent function that does not allow
// the result to be unwrapped.
func Wrap(errp *error, format string, args ...any) {
	if *errp != nil {
		*errp = fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), *errp)
	}
}

// WrapStack is like Wrap, but adds a stack trace if there isn't one already.
func WrapStack(errp *error, format string, args ...any) {
	if *errp != nil {
		if se := (*StackError)(nil); !errors.As(*errp, &se) {
			*errp = NewStackError(*errp)
		}
		Wrap(errp, format, args...)
	}
}

// StackError wraps an error and adds a stack trace.
type StackError struct {
	Stack []byte
	err   error
}

// NewStackError returns a StackError, capturing a stack trace.
func NewStackError(err error) *StackError {
	// Limit the stack trace to 16K. Same value used in the errorreporting client,
	// cloud.google.com/go@v0.66.0/errorreporting/errors.go.
	var buf [16 * 1024]byte
	n := runtime.Stack(buf[:], false)
	return &StackError{
		err:   err,
		Stack: buf[:n],
	}
}

func (e *StackError) Error() string {
	return e.err.Error() // ignore the stack
}

func (e *StackError) Unwrap() error {
	return e.err
}

// WrapAndReport calls Wrap followed by Report.
func WrapAndReport(errp *error, format string, args ...any) {
	Wrap(errp, format, args...)
	if *errp != nil {
		Report(*errp)
	}
}

var reporter Reporter

// SetReporter the Reporter to use, for use by Report.
func SetReporter(r Reporter) {
	reporter = r
}

// Reporter is an interface used for reporting errors.
type Reporter interface {
	Report(err error, req *http.Request, stack []byte)
}

// Report uses the Reporter to report an error.
func Report(err error) {
	if reporter != nil {
		reporter.Report(err, nil, nil)
	}
}
