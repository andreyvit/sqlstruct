package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/andreyvit/sqlstruct"
)

var (
	in      = flag.String("in", "", "input package name(s)")
	outFile = flag.String("out", "", "output file name")
	pkgName = flag.String("pkg", "", "package name")
	pub     = flag.Bool("pub", false, "generate published funcs and types")
)

var (
	stdLog = log.New(os.Stderr, "", 0)
)

const (
	databaseSQL = "database/sql"
)

func main() {
	flag.Parse()

	err := run()
	if err != nil {
		stdLog.Fatalf("** %+v", err)
	}
}

func run() error {
	if *in == "" {
		return fmt.Errorf("-in not specified")
	}

	inPkgs := strings.Split(*in, ",")

	outData, genErr := sqlstruct.GeneratePkg(inPkgs, sqlstruct.Options{
		Logger:    stdLog,
		PkgName:   *pkgName,
		Published: *pub,
	})
	if genErr != nil && outData == nil {
		return genErr
	}

	if *outFile == "" {
		fmt.Fprintln(os.Stdout, string(outData))
	} else {
		err := ioutil.WriteFile(*outFile, outData, 0644)
		if err != nil {
			return err
		}
	}

	return genErr
}
