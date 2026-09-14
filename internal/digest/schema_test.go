package digest

import "testing"

func TestParseFileMap_Valid(t *testing.T) {
	in := []byte(`{"summary":"does things","map":[{"lines":"1-40","kind":"imports"},{"lines":"41","kind":"var block"}],"symbols":["A"],"notes":"approx"}`)
	fm, err := ParseFileMap(in)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Summary != "does things" || len(fm.Map) != 2 {
		t.Fatalf("got %+v", fm)
	}
}

func TestParseFileMap_Invalid(t *testing.T) {
	for name, in := range map[string]string{
		"empty summary": `{"summary":"","map":[{"lines":"1-2","kind":"x"}]}`,
		"no map":        `{"summary":"s","map":[]}`,
		"bad range":     `{"summary":"s","map":[{"lines":"forty","kind":"x"}]}`,
		"not json":      `nope`,
	} {
		if _, err := ParseFileMap([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseRun_Valid(t *testing.T) {
	r, err := ParseRun([]byte(`{"summary":"2 failed","key_lines":["FAIL x"],"exit_code":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 1 {
		t.Fatalf("got %+v", r)
	}
}
