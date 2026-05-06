package pipeline

import "testing"

func TestMergeCommitKeysFromEnv_fromCommitID(t *testing.T) {
	t.Parallel()
	src := map[string]string{"COMMIT_ID": "abc123def"}
	dst := map[string]string{}
	mergeCommitKeysFromEnv(src, dst)
	if got := dst["CI_COMMIT_SHA"]; got != "abc123def" {
		t.Fatalf("CI_COMMIT_SHA=%q", got)
	}
	if got := dst["COMMIT_ID"]; got != "abc123def" {
		t.Fatalf("COMMIT_ID=%q", got)
	}
	if got := dst["COMMIT_ID_SHA"]; got != "abc123def" {
		t.Fatalf("COMMIT_ID_SHA=%q", got)
	}
}

func TestMergeCommitKeysFromEnv_prefersCICommitSHA(t *testing.T) {
	t.Parallel()
	src := map[string]string{
		"CI_COMMIT_SHA": "fullsha",
		"COMMIT_ID":     "other",
	}
	dst := map[string]string{}
	mergeCommitKeysFromEnv(src, dst)
	if dst["COMMIT_ID"] != "fullsha" {
		t.Fatalf("expected CI_COMMIT_SHA to win, dst=%v", dst)
	}
}

func TestMergeCommitKeysFromEnv_noopWhenEmpty(t *testing.T) {
	t.Parallel()
	src := map[string]string{"FOO": "bar"}
	dst := map[string]string{"EXIST": "1"}
	mergeCommitKeysFromEnv(src, dst)
	if len(dst) != 1 || dst["EXIST"] != "1" {
		t.Fatalf("dst=%v", dst)
	}
}
