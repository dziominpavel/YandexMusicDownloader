package quality

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	path := `c:\music\a.mp3`
	j.Decline(path, Declined{TrackID: "123", MatchedTitle: "Song", DeclinedAt: now})
	if !j.IsDeclined(path, "123") {
		t.Fatal("declined match must be remembered")
	}
	if j.IsDeclined(path, "456") {
		t.Fatal("another candidate must not be declined")
	}
	if !j.IsDeclined(path, "123") {
		t.Fatal("decline must not expire")
	}

	j.Decline(path, Declined{TrackID: "456", DeclinedAt: now.Add(time.Hour)})
	if !j.IsDeclined(path, "123") || !j.IsDeclined(path, "456") {
		t.Fatal("both rejected candidates must be kept")
	}
	j.Decline(path, Declined{TrackID: "123", MatchedTitle: "Updated", DeclinedAt: now.Add(2 * time.Hour)})
	list := j.DeclinedList()
	if len(list) != 2 {
		t.Fatalf("want two rejections, got %+v", list)
	}
	if list[0].TrackID != "123" || list[0].MatchedTitle != "Updated" {
		t.Fatalf("newest rejection must replace the old one: %+v", list)
	}

	d, ok := j.Undecline(path, "123")
	if !ok || d.TrackID != "123" || d.MatchedTitle != "Updated" {
		t.Fatalf("undo must return the stored rejection: %+v ok=%v", d, ok)
	}
	if j.IsDeclined(path, "123") {
		t.Fatal("undone rejection must be gone")
	}
	if !j.IsDeclined(path, "456") {
		t.Fatal("undo must not remove another candidate")
	}
}

func TestJournalRejectionsSurvivePlusAndPrune(t *testing.T) {
	j := NewJournal("")
	path := `c:\music\a.mp3`
	j.SetAccount(1, false)
	j.Decline(path, Declined{TrackID: "42", DeclinedAt: testTime()})
	if !j.SetAccount(1, true) {
		t.Fatal("false→true Plus transition must reset entries")
	}
	if !j.IsDeclined(path, "42") {
		t.Fatal("Plus transition must keep rejections")
	}
	if !j.SetAccount(2, true) {
		t.Fatal("changed uid must reset the journal")
	}
	if j.IsDeclined(path, "42") {
		t.Fatal("changed uid must clear rejections")
	}

	j.Decline(path, Declined{TrackID: "42", DeclinedAt: testTime()})
	j.Prune(map[string]bool{}, map[string]bool{})
	if j.IsDeclined(path, "42") {
		t.Fatal("prune must drop rejections for missing local files")
	}
}

func TestJournalDeclinedPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flac-quality.json")
	now := testTime()
	j := NewJournal(path)
	j.Decline(`c:\music\a.mp3`, Declined{
		TrackID: "42", LocalLabel: "Artist - Song.mp3",
		MatchedArtist: "Artist", MatchedTitle: "Song (Remix)",
		DeclinedAt: now,
	})
	j.Save()

	loaded := NewJournal(path)
	if !loaded.IsDeclined(`c:\music\a.mp3`, "42") {
		t.Fatal("rejection must survive a reload")
	}
	list := loaded.DeclinedList()
	if len(list) != 1 || list[0].MatchedTitle != "Song (Remix)" {
		t.Fatalf("rejection metadata lost: %+v", list)
	}
}

func TestJournalMigratesV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flac-quality.json")
	raw := `{
		"version": 1,
		"entries": {
			"file:c:\\music\\a.mp3": {
				"status": "lossless",
				"codec": "flac",
				"match_source": "search",
				"matched_track_id": "42",
				"matched_artist": "Artist",
				"matched_title": "Song"
			}
		},
		"declined": {
			"c:\\music\\a.mp3": {"track_id": "42", "until": "2099-01-01T00:00:00Z"},
			"c:\\music\\old.mp3": {"track_id": "7", "until": "2000-01-01T00:00:00Z"}
		}
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	j := NewJournal(path)
	if !j.IsDeclined(`c:\music\a.mp3`, "42") {
		t.Fatal("active v1 rejection must migrate")
	}
	if j.IsDeclined(`c:\music\old.mp3`, "7") {
		t.Fatal("expired v1 rejection must be dropped")
	}
	list := j.DeclinedList()
	if len(list) != 1 || list[0].MatchedArtist != "Artist" || list[0].MatchedTitle != "Song" {
		t.Fatalf("v1 metadata must migrate from the file entry: %+v", list)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `"version":2`) {
		t.Fatalf("migrated journal must be written as v2: %s", saved)
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
