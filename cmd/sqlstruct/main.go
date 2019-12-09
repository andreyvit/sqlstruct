package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/andreyvit/sqlstruct"
)

var (
	inFile  = flag.String("in", "", "input file or directory name")
	inPkg   = flag.String("inPkg", "", "input package name")
	outFile = flag.String("out", "", "output file name")
	pkgName = flag.String("pkg", "", "package name")
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
	if *inFile == "" {
		return fmt.Errorf("input file not specified")
	}

	inData, err := ioutil.ReadFile(*inFile)
	if err != nil {
		return err
	}

	outData, err := sqlstruct.Generate(inData, sqlstruct.Options{
		Logger:        stdLog,
		InputFileName: *inFile,
		InputPkgName:  *inPkg,
		PkgName:       *pkgName,
	})
	if err != nil {
		return err
	}

	if *outFile == "" {
		fmt.Fprintln(os.Stdout, string(outData))
	} else {
		err = ioutil.WriteFile(*outFile, outData, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}
