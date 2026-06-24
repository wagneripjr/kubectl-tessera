package cli

import (
	"bytes"
	"strings"
	"testing"
)

func executeRoot(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := newRootCmd(BuildInfo{Version: "v0.0.0-test", Commit: "test", Date: "test"})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), err
}

func TestBareInvocationPrintsHelpAndExitsZero(t *testing.T) {
	out, err := executeRoot(t)
	if err != nil {
		t.Fatalf("bare invocation must exit 0 (a discovery gesture, not a failed mint); got error: %v", err)
	}
	for _, want := range []string{
		"Usage:",
		"kubectl tessera [flags]",
		"Examples:",
		"kubectl tessera --resource pods",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bare-invocation help is missing %q\n--- help output ---\n%s", want, out)
		}
	}
}

func TestFlaggedButIncompleteInvocationStillRequiresResource(t *testing.T) {
	out, err := executeRoot(t, "-n", "prod")

	if err == nil {
		t.Fatalf("a set flag must fall through to the mint path, not the help shortcut; got nil error\n--- output ---\n%s", out)
	}
	if !strings.Contains(err.Error(), "--resource is required") {
		t.Errorf("want %q, got %q", "--resource is required", err.Error())
	}
	if strings.Contains(out, "Examples:") {
		t.Errorf("an incomplete-but-flagged invocation must not print help; got:\n%s", out)
	}
}

func TestStrayPositionalArgFallsThrough(t *testing.T) {
	out, err := executeRoot(t, "bogus")

	if err == nil {
		t.Fatalf("a stray positional arg must not be swallowed into help; got nil error\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "Examples:") {
		t.Errorf("a stray positional arg must not print help; got:\n%s", out)
	}
}

func TestSubcommandsCarryUsageExamples(t *testing.T) {
	for _, sub := range []string{"version", "gc", "ls"} {
		t.Run(sub, func(t *testing.T) {
			out, err := executeRoot(t, sub, "--help")
			if err != nil {
				t.Fatalf("%s --help should exit 0; got: %v", sub, err)
			}
			if !strings.Contains(out, "Examples:") {
				t.Errorf("%s --help is missing an Examples section\n--- output ---\n%s", sub, out)
			}
			if !strings.Contains(out, "kubectl tessera "+sub) {
				t.Errorf("%s --help example should invoke %q\n--- output ---\n%s",
					sub, "kubectl tessera "+sub, out)
			}
		})
	}
}
