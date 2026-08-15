package confirmation

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskYes(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		p := New(strings.NewReader(input), &bytes.Buffer{})
		got, err := p.Ask("Proceed?")
		if err != nil {
			t.Fatal(err)
		}
		if !got {
			t.Errorf("expected true for input %q", input)
		}
	}
}

func TestAskNo(t *testing.T) {
	for _, input := range []string{"n\n", "N\n", "no\n", "NO\n", "\n", "  \n"} {
		p := New(strings.NewReader(input), &bytes.Buffer{})
		got, err := p.Ask("Proceed?")
		if err != nil {
			t.Fatal(err)
		}
		if got {
			t.Errorf("expected false for input %q", input)
		}
	}
}

func TestAskInvalidThenYes(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("maybe\ny\n"), &out)
	got, err := p.Ask("Proceed?")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("expected true after invalid then y")
	}
	if !strings.Contains(out.String(), "Please answer y or n") {
		t.Fatalf("expected error message, got %q", out.String())
	}
}

func TestAskStringDefault(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("\n"), &out)
	got, err := p.AskStringDefault("Host", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "example.com" {
		t.Fatalf("expected default, got %q", got)
	}
}

func TestAskStringOverridesDefault(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("new.example.com\n"), &out)
	got, err := p.AskStringDefault("Host", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "new.example.com" {
		t.Fatalf("expected override, got %q", got)
	}
}

func TestAskStringNoDefault(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("myprofile\n"), &out)
	got, err := p.AskStringDefault("Profile name", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "myprofile" {
		t.Fatalf("expected input, got %q", got)
	}
}

func TestAskIntDefault(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("\n"), &out)
	got, err := p.AskIntDefault("Port", 22)
	if err != nil {
		t.Fatal(err)
	}
	if got != 22 {
		t.Fatalf("expected 22, got %d", got)
	}
}

func TestAskIntOverride(t *testing.T) {
	var out bytes.Buffer
	p := New(strings.NewReader("2222\n"), &out)
	got, err := p.AskIntDefault("Port", 22)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2222 {
		t.Fatalf("expected 2222, got %d", got)
	}
}

func TestAskIntInvalid(t *testing.T) {
	p := New(strings.NewReader("abc\n"), &bytes.Buffer{})
	if _, err := p.AskIntDefault("Port", 22); err == nil {
		t.Fatal("expected error for invalid int")
	}
}
