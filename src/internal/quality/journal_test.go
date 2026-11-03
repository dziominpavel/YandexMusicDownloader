package quality

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testTime() time.Time {
	return time.Date(2026, 11, 3, 18, 0, 0, 0, time.Local)
}

func TestJournalRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flac-quality.json")
	now := testTime()

	j := NewJournal(path)
	j.SetAccount(42, true)
	j.Put("1", Entry{
		Status: StatusNoLossless, Codec: "mp3",
		CheckedAt: now, NextCheck: now.Add(NoLosslessTTL),
	})
	j.Begin(now)
	j.Finish(now, 1, nil)
	j.Save()

	loaded := NewJournal(path)
	e, ok := loaded.Get("1")
	if !ok || e.Status != StatusNoLossless || e.Codec != "mp3" {
		t.Fatalf("entry lost after reload: %+v ok=%v", e, ok)
	}
	st := loaded.Status()
	if !st.LastAttempt.Equal(now) || !st.LastCompleted.Equal(now) || st.Found != 1 || st.Error != "" {
		t.Fatalf("status lost after reload: %+v", st)
	}
	if loaded.DueCheck(now.Add(6 * 24 * time.Hour)) {
		t.Fatal("auto-check must not be due before 7 days")
	}
	if !loaded.DueCheck(now.Add(7 * 24 * time.Hour)) {
		t.Fatal("auto-check must be due after 7 days")
	}
}

func TestJournalTTL(t *testing.T) {
	j := NewJournal("")
	now := testTime()
	j.Put("1", Entry{Status: StatusNoLossless, NextCheck: now.Add(NoLosslessTTL)})
	if j.Due("1", now.Add(29*24*time.Hour)) {
		t.Fatal("entry must not be due before its TTL")
	}
	if !j.Due("1", now.Add(NoLosslessTTL)) {
		t.Fatal("entry must be due at its TTL")
	}
	if !j.Due("missing", now) {
		t.Fatal("missing entry must always be due")
	}
}

func TestJournalAccountAndPlusReset(t *testing.T) {
	j := NewJournal("")
	j.SetAccount(1, true)
	j.Put("1", Entry{Status: StatusNoLossless})
	if !j.SetAccount(2, true) {
		t.Fatal("changed uid must reset the journal")
	}
	if _, ok := j.Get("1"); ok {
		t.Fatal("old account entries must be gone")
	}

	j.SetAccount(2, false)
	j.Put("2", Entry{Status: StatusNoLossless})
	if !j.SetAccount(2, true) {
		t.Fatal("false→true Plus transition must reset the journal")
	}
	if _, ok := j.Get("2"); ok {
		t.Fatal("no-lossless entries must be cleared when Plus appears")
	}
	if !j.DueCheck(testTime()) {
		t.Fatal("Plus transition must make the next check due")
	}
}

func TestJournalCorruptIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flac-quality.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := NewJournal(path)
	if _, ok := j.Get("1"); ok {
		t.Fatal("corrupt journal must load empty")
	}
}

func TestJournalDeclined(t *testing.T) {
	j := NewJournal("")
	now := testTime()
	j.Decline(`c:\music\a.mp3`, "123", now)
	if !j.IsDeclined(`c:\music\a.mp3`, "123", now.Add(time.Hour)) {
		t.Fatal("declined match must be remembered")
	}
	if j.IsDeclined(`c:\music\a.mp3`, "456", now) {
		t.Fatal("another candidate must not be declined")
	}
	if j.IsDeclined(`c:\music\a.mp3`, "123", now.Add(DeclinedTTL)) {
		t.Fatal("decline must expire")
	}
}

func TestJournalFinishErrorThenSuccess(t *testing.T) {
	j := NewJournal("")
	now := testTime()
	j.Begin(now)
	j.Finish(now, 0, errors.New("boom"))
	st := j.Status()
	if st.Error != "boom" || !st.LastCompleted.IsZero() {
		t.Fatalf("error must not look like a completed check: %+v", st)
	}
	if !j.DueCheck(now.Add(RetryAfterError)) {
		t.Fatal("error must retry after the short TTL")
	}

	j.Finish(now, 2, nil)
	st = j.Status()
	if st.Error != "" || st.Found != 2 || !st.LastCompleted.Equal(now) {
		t.Fatalf("success must clear the error and store the result: %+v", st)
	}
}

func TestDue(t *testing.T) {
	now := testTime()
	if !Due(time.Time{}, now, AutoCheckInterval) {
		t.Fatal("zero date is always due")
	}
	if Due(now.Add(-6*24*time.Hour), now, AutoCheckInterval) {
		t.Fatal("must not be due before the interval")
	}
	if !Due(now.Add(-AutoCheckInterval), now, AutoCheckInterval) {
		t.Fatal("must be due at the interval")
	}
}
