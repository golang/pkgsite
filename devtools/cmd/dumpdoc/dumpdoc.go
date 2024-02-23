// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The dumpdoc command writes documentation and readmes for packages
// in search_documents to a gob file.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/printer"
	"go/token"
	"io"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/log"
)

var (
	truncate     = flag.Int("t", 0, "(only for read) truncate long strings to the given length")
	minImporters = flag.Int("i", 1, "(only for write) include only packages with at least this many importers")
)

func main() {
	ctx := context.Background()
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage:\n")
		fmt.Fprintf(out, "  %s [flags] write FILE\n", os.Args[0])
		fmt.Fprintf(out, "  %s [flags] read FILE\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.Arg(1) == "" {
		flag.Usage()
		os.Exit(1)
	}
	if err := run(ctx, flag.Arg(0), flag.Arg(1)); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, cmd, filename string) error {
	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		return err
	}
	switch cmd {
	case "write":
		db, err := database.Open("pgx", cfg.DBConnInfo(), "dumpdoc")
		if err != nil {
			return err
		}
		defer db.Close()
		return write(ctx, db, filename)
	case "read":
		return read(filename)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func write(ctx context.Context, db *database.DB, filename string) error {
	query := fmt.Sprintf(`
		SELECT s.package_path, s.module_path, s.version, s.imported_by_count,
			   r.file_path, r.contents,
			   d.source
		FROM search_documents s
		LEFT JOIN readmes r USING (unit_id)
		INNER JOIN documentation d USING (unit_id)
		WHERE (d.goos = 'all' OR d.goos = 'linux')
		AND imported_by_count >= %d
	`, *minImporters)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(f)
	n := 0
	err = db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var pd PackageDoc
		var source []byte
		err := rows.Scan(&pd.ImportPath, &pd.ModulePath, &pd.Version, &pd.NumImporters,
			&pd.ReadmeFilename, &pd.ReadmeContents, &source)
		if err != nil {
			return err
		}
		if err := populateDoc(&pd, source); err != nil {
			return err
		}
		if err := enc.Encode(pd); err != nil {
			return err
		}
		n++
		if n%1000 == 0 {
			fmt.Printf("%d\n", n)
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("wrote %d packages.\n", n)
	return f.Close()
}

func populateDoc(pd *PackageDoc, source []byte) error {
	gpkg, err := godoc.DecodePackage(source)
	if err != nil {
		return err
	}
	innerPath := strings.TrimPrefix(pd.ImportPath, pd.ModulePath+"/")
	modInfo := &godoc.ModuleInfo{ModulePath: pd.ModulePath, ResolvedVersion: pd.Version}
	dpkg, err := gpkg.DocPackage(innerPath, modInfo)
	if err != nil {
		return err
	}
	pd.PackageDoc = dpkg.Doc
	var sds []SymbolDoc
	for _, v := range dpkg.Consts {
		sds = append(sds, valueSymbolDoc(gpkg.Fset, v))
	}
	for _, v := range dpkg.Vars {
		sds = append(sds, valueSymbolDoc(gpkg.Fset, v))
	}
	for _, t := range dpkg.Types {
		sd := SymbolDoc{
			Names: []string{t.Name},
			Decl:  formatDecl(gpkg.Fset, t.Decl),
			Doc:   t.Doc,
		}
		sds = append(sds, sd)
		for _, v := range t.Consts {
			sds = append(sds, valueSymbolDoc(gpkg.Fset, v))
		}
		for _, v := range t.Vars {
			sds = append(sds, valueSymbolDoc(gpkg.Fset, v))
		}
		for _, f := range t.Funcs {
			// No prefix: these are top-level functions that return the type.
			sds = append(sds, functionSymbolDoc("", gpkg.Fset, f))
		}
		for _, f := range t.Methods {
			sds = append(sds, functionSymbolDoc(t.Name, gpkg.Fset, f))
		}
		// TODO: Examples
	}
	for _, f := range dpkg.Funcs {
		sds = append(sds, functionSymbolDoc("", gpkg.Fset, f))
		// TODO:Examples
	}
	pd.SymbolDocs = sds
	return nil
}

func valueSymbolDoc(fset *token.FileSet, v *doc.Value) SymbolDoc {
	return SymbolDoc{
		Names: v.Names,
		Decl:  formatDecl(fset, v.Decl),
		Doc:   v.Doc,
	}
}

func functionSymbolDoc(prefix string, fset *token.FileSet, f *doc.Func) SymbolDoc {
	if prefix != "" {
		prefix += "."
	}
	return SymbolDoc{
		Names: []string{prefix + f.Name},
		Decl:  formatDecl(fset, f.Decl),
		Doc:   f.Doc,
	}

}

func formatDecl(fset *token.FileSet, decl ast.Decl) string {
	p := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	var b bytes.Buffer
	p.Fprint(&b, fset, decl)
	return b.String()
}

func read(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := gob.NewDecoder(f)
	for {
		var pd PackageDoc
		err := dec.Decode(&pd)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		pd.Show()
	}
}

func (pd PackageDoc) Show() {
	pd.PackageDoc = trunc(pd.PackageDoc)
	fmt.Printf("%s (%s@%s):\n", pd.ImportPath, pd.ModulePath, pd.Version)
	fmt.Printf("    %d importers\n", pd.NumImporters)
	fmt.Printf("     pkg doc: %q\n", pd.PackageDoc)
	if pd.ReadmeFilename != nil && pd.ReadmeContents != nil {
		*pd.ReadmeContents = trunc(*pd.ReadmeContents)
		fmt.Printf("     readme (from %s): %q\n", *pd.ReadmeFilename, *pd.ReadmeContents)
	}
	fmt.Printf("    symbols\n:")
	for _, sd := range pd.SymbolDocs {
		fmt.Printf("\tNames: %v\n", sd.Names)
		fmt.Printf("\tDecl: %s\n", sd.Decl)
		fmt.Printf("\tDoc: %q\n", trunc(sd.Doc))
		fmt.Println()
	}
}

func trunc(s string) string {
	if *truncate <= 0 {
		return s
	}
	if len(s) < *truncate {
		return s
	}
	return s[:*truncate]
}
