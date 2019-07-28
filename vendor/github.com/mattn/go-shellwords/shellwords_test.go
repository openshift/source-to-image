package shellwords

import (
	"os"
	"reflect"
	"testing"
)

var testcases = []struct {
	line     string
	expected []string
}{
	{`var --bar=baz`, []string{`var`, `--bar=baz`}},
	{`var --bar="baz"`, []string{`var`, `--bar=baz`}},
	{`var "--bar=baz"`, []string{`var`, `--bar=baz`}},
	{`var "--bar='baz'"`, []string{`var`, `--bar='baz'`}},
	{"var --bar=`baz`", []string{`var`, "--bar=`baz`"}},
	{`var "--bar=\"baz'"`, []string{`var`, `--bar="baz'`}},
	{`var "--bar=\'baz\'"`, []string{`var`, `--bar='baz'`}},
	{`var --bar='\'`, []string{`var`, `--bar=\`}},
	{`var "--bar baz"`, []string{`var`, `--bar baz`}},
	{`var --"bar baz"`, []string{`var`, `--bar baz`}},
	{`var  --"bar baz"`, []string{`var`, `--bar baz`}},
}

func TestSimple(t *testing.T) {
	for _, testcase := range testcases {
		args, err := Parse(testcase.line)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(args, testcase.expected) {
			t.Fatalf("Expected %#v, but %#v:", testcase.expected, args)
		}
	}
}

func TestError(t *testing.T) {
	_, err := Parse("foo '")
	if err == nil {
		t.Fatal("Should be an error")
	}
	_, err = Parse(`foo "`)
	if err == nil {
		t.Fatal("Should be an error")
	}

	_, err = Parse("foo `")
	if err == nil {
		t.Fatal("Should be an error")
	}
}

func TestLastSpace(t *testing.T) {
	args, err := Parse("foo bar\\  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 {
		t.Fatal("Should have two elements")
	}
	if args[0] != "foo" {
		t.Fatal("1st element should be `foo`")
	}
	if args[1] != "bar " {
		t.Fatal("1st element should be `bar `")
	}
}

func TestBacktick(t *testing.T) {
	goversion, err := shellRun("go version")
	if err != nil {
		t.Fatal(err)
	}

	parser := NewParser()
	parser.ParseBacktick = true
	args, err := parser.Parse("echo `go version`")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"echo", goversion}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	args, err = parser.Parse(`echo $(echo foo)`)
	if err != nil {
		t.Fatal(err)
	}
	expected = []string{"echo", "foo"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	args, err = parser.Parse(`echo bar=$(echo 200)cm`)
	if err != nil {
		t.Fatal(err)
	}
	expected = []string{"echo", "bar=200cm"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	parser.ParseBacktick = false
	args, err = parser.Parse(`echo $(echo "foo")`)
	expected = []string{"echo", `$(echo "foo")`}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
	args, err = parser.Parse("echo $(`echo1)")
	if err != nil {
		t.Fatal(err)
	}
	expected = []string{"echo", "$(`echo1)"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
}

func TestBacktickError(t *testing.T) {
	parser := NewParser()
	parser.ParseBacktick = true
	_, err := parser.Parse("echo `go Version`")
	if err == nil {
		t.Fatal("Should be an error")
	}
	expected := "exit status 2:go Version: unknown command\nRun 'go help' for usage.\n"
	if expected != err.Error() {
		t.Fatalf("Expected %q, but %q", expected, err.Error())
	}
	_, err = parser.Parse(`echo $(echo1)`)
	if err == nil {
		t.Fatal("Should be an error")
	}
	_, err = parser.Parse(`echo FOO=$(echo1)`)
	if err == nil {
		t.Fatal("Should be an error")
	}
	_, err = parser.Parse(`echo $(echo1`)
	if err == nil {
		t.Fatal("Should be an error")
	}
	_, err = parser.Parse(`echo $ (echo1`)
	if err == nil {
		t.Fatal("Should be an error")
	}
	_, err = parser.Parse(`echo (echo1`)
	if err == nil {
		t.Fatal("Should be an error")
	}
	_, err = parser.Parse(`echo )echo1`)
	if err == nil {
		t.Fatal("Should be an error")
	}
}

func TestEnv(t *testing.T) {
	os.Setenv("FOO", "bar")

	parser := NewParser()
	parser.ParseEnv = true
	args, err := parser.Parse("echo $FOO")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"echo", "bar"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
}

func TestCustomEnv(t *testing.T) {
	parser := NewParser()
	parser.ParseEnv = true
	parser.Getenv = func(k string) string { return map[string]string{"FOO": "baz"}[k] }
	args, err := parser.Parse("echo $FOO")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"echo", "baz"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
}

func TestNoEnv(t *testing.T) {
	parser := NewParser()
	parser.ParseEnv = true
	args, err := parser.Parse("echo $BAR")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"echo", ""}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
}

func TestDupEnv(t *testing.T) {
	os.Setenv("FOO", "bar")
	os.Setenv("FOO_BAR", "baz")

	parser := NewParser()
	parser.ParseEnv = true
	args, err := parser.Parse("echo $$FOO$")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"echo", "$bar$"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	args, err = parser.Parse("echo $${FOO_BAR}$")
	if err != nil {
		t.Fatal(err)
	}
	expected = []string{"echo", "$baz$"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
}

func TestHaveMore(t *testing.T) {
	parser := NewParser()
	parser.ParseEnv = true

	line := "echo foo; seq 1 10"
	args, err := parser.Parse(line)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected := []string{"echo", "foo"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	if parser.Position == 0 {
		t.Fatalf("Commands should be remaining")
	}

	line = string([]rune(line)[parser.Position+1:])
	args, err = parser.Parse(line)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected = []string{"seq", "1", "10"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	if parser.Position > 0 {
		t.Fatalf("Commands should not be remaining")
	}
}

func TestHaveRedirect(t *testing.T) {
	parser := NewParser()
	parser.ParseEnv = true

	line := "ls -la 2>foo"
	args, err := parser.Parse(line)
	if err != nil {
		t.Fatalf(err.Error())
	}
	expected := []string{"ls", "-la"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}

	if parser.Position == 0 {
		t.Fatalf("Commands should be remaining")
	}
}

func TestBackquoteInFlag(t *testing.T) {
	parser := NewParser()
	parser.ParseBacktick = true
	args, err := parser.Parse("cmd -flag=`echo val1` -flag=val2")
	if err != nil {
		panic(err)
	}
	expected := []string{"cmd", "-flag=val1", "-flag=val2"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("Expected %#v, but %#v:", expected, args)
	}
}
