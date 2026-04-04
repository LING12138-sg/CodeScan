package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codescan/internal/release"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "check":
		runCheck(os.Args[2:])
	case "export":
		runExport(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runCheck(args []string) {
	flags := flag.NewFlagSet("check", flag.ExitOnError)
	root := flags.String("root", ".", "Workspace root to scan")
	flags.Parse(args)

	files, err := release.CollectReleaseFiles(*root)
	if err != nil {
		exitWithError(err)
	}

	findings, err := release.ScanFiles(files)
	if err != nil {
		exitWithError(err)
	}

	fmt.Printf("Release scan scope: %d files\n", len(files))
	if len(findings) == 0 {
		fmt.Println("No publishable-file secret findings detected.")
		return
	}

	fmt.Println("Secret findings in publishable files:")
	for _, finding := range findings {
		fmt.Printf(" - %s:%d [%s] %s\n", finding.Path, finding.Line, finding.Rule, finding.Excerpt)
	}
	os.Exit(1)
}

func runExport(args []string) {
	flags := flag.NewFlagSet("export", flag.ExitOnError)
	root := flags.String("root", ".", "Workspace root to export")
	out := flags.String("out", filepath.Join("release", "CodeScan-open-source.zip"), "Output zip path")
	flags.Parse(args)

	files, err := release.CollectReleaseFiles(*root)
	if err != nil {
		exitWithError(err)
	}

	findings, err := release.ScanFiles(files)
	if err != nil {
		exitWithError(err)
	}
	if len(findings) > 0 {
		fmt.Println("Refusing to export because publishable files contain secrets:")
		for _, finding := range findings {
			fmt.Printf(" - %s:%d [%s] %s\n", finding.Path, finding.Line, finding.Rule, finding.Excerpt)
		}
		os.Exit(1)
	}

	if err := release.CreateArchive(files, *out); err != nil {
		exitWithError(err)
	}

	result, err := release.ValidateArchive(*out)
	if err != nil {
		exitWithError(err)
	}
	if len(result.UnexpectedEntries) > 0 || len(result.Findings) > 0 {
		fmt.Printf("Exported archive %s failed validation.\n", *out)
		for _, entry := range result.UnexpectedEntries {
			fmt.Printf(" - unexpected entry: %s\n", entry)
		}
		for _, finding := range result.Findings {
			fmt.Printf(" - %s:%d [%s] %s\n", finding.Path, finding.Line, finding.Rule, finding.Excerpt)
		}
		os.Exit(1)
	}

	fmt.Printf("Release archive created: %s\n", *out)
	fmt.Printf("Validated %d archive entries.\n", result.FileCount)
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  go run ./cmd/release check")
	fmt.Println("  go run ./cmd/release export -out release/CodeScan-open-source.zip")
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
