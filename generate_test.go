package sqlstruct

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/andreyvit/diff"
)

func TestGenerate(t *testing.T) {
	test(t, "foo", "ok", Options{})
}

func test(t *testing.T, base string, suffix string, opt Options) {
	input := base + ".go"
	output := base + "_" + suffix + ".go"

	opt.Logger = testingLogger{t}
	opt.Published = false

	t.Run(base+"_"+suffix, func(t *testing.T) {
		a := generate(input, opt)
		e := string(bytes.TrimSpace(load(output)))
		if a != e {
			t.Skip()
			t.Errorf("* Generate(%s) != %s:\n\n%s", input, output, diff.LineDiff(e, a))
		} else {
			t.Logf("✓ Generate(%s) == %s", input, output)
		}
	})
}

func generate(fn string, opt Options) string {
	result, err := Generate(load(fn), opt)
	if err != nil {
		panic(err)
	}
	return string(bytes.TrimSpace(result))
}

func load(fn string) []byte {
	data, err := ioutil.ReadFile("testdata/" + fn)
	if err != nil {
		panic(err)
	}
	return data
}

func trim(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\t\t", ""))
}

type testingLogger struct {
	tb testing.TB
}

func (logger testingLogger) Printf(format string, v ...interface{}) {
	logger.tb.Logf(format, v...)
}
