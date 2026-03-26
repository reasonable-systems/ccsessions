package ui

import (
	"testing"
	"time"

	"ccsessions/internal/claude"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFindDetailMatches_CaseInsensitiveAcrossLines(t *testing.T) {
	lines := []detailLine{
		{text: "Alpha beta", kind: detailLinePlain},
		{text: "beta BETA", kind: detailLinePlain},
		{text: "gamma", kind: detailLinePlain},
	}

	matches := findDetailMatches(lines, "beta")
	if got, want := len(matches), 3; got != want {
		t.Fatalf("len(matches) = %d, want %d", got, want)
	}

	if matches[0].line != 0 || matches[0].start != 6 || matches[0].end != 10 {
		t.Fatalf("first match = %+v, want line 0 start 6 end 10", matches[0])
	}

	if matches[2].line != 1 || matches[2].start != 5 || matches[2].end != 9 {
		t.Fatalf("third match = %+v, want line 1 start 5 end 9", matches[2])
	}
}

func TestDetailSearchLifecycleAndNavigation(t *testing.T) {
	model := testModelWithSessions([]claude.Session{
		testSession("session-1", []claude.Entry{
			{Kind: claude.EntryAssistantText, Content: "alpha line"},
			{Kind: claude.EntryAssistantText, Content: "beta line"},
			{Kind: claude.EntryAssistantText, Content: "beta again"},
		}),
	})

	model.focus = focusDetails

	model = pressRunes(t, model, '/')
	if !model.detailSearchEditing {
		t.Fatal("expected detail search to enter editing mode")
	}

	model = pressRunes(t, model, 'b', 'e', 't', 'a')
	if !model.detailSearchActive {
		t.Fatal("expected detail search to be active after typing")
	}
	if got, want := len(model.detailMatches), 2; got != want {
		t.Fatalf("len(detailMatches) = %d, want %d", got, want)
	}
	if got, want := model.details.YOffset, expectedScrollOffset(model, 0); got != want {
		t.Fatalf("details.YOffset = %d, want %d", got, want)
	}

	model = pressKey(t, model, tea.KeyDown)
	if got, want := model.activeDetailMatch, 1; got != want {
		t.Fatalf("activeDetailMatch = %d, want %d", got, want)
	}
	if got, want := model.details.YOffset, expectedScrollOffset(model, 1); got != want {
		t.Fatalf("details.YOffset after down = %d, want %d", got, want)
	}

	model = pressKey(t, model, tea.KeyEnter)
	if model.detailSearchEditing {
		t.Fatal("expected Enter to end editing mode")
	}
	if !model.detailSearchActive {
		t.Fatal("expected search to remain active after Enter")
	}

	model = pressKey(t, model, tea.KeyEsc)
	if model.detailSearchActive {
		t.Fatal("expected Esc to clear active search")
	}
	if model.detailSearch.Value() != "" {
		t.Fatalf("detailSearch.Value() = %q, want empty", model.detailSearch.Value())
	}
	if got := len(model.detailMatches); got != 0 {
		t.Fatalf("len(detailMatches) = %d, want 0", got)
	}
}

func TestDetailSearchRecomputesOnSessionChangeAndResize(t *testing.T) {
	model := testModelWithSessions([]claude.Session{
		testSession("session-1", []claude.Entry{
			{Kind: claude.EntryAssistantText, Content: "needle one"},
		}),
		testSession("session-2", []claude.Entry{
			{Kind: claude.EntryAssistantText, Content: "second session has needle twice needle"},
		}),
	})

	model.focus = focusDetails
	model.detailSearch.SetValue("needle")
	model.detailSearchActive = true
	model.recomputeDetailMatches(true)
	if got, want := len(model.detailMatches), 1; got != want {
		t.Fatalf("initial len(detailMatches) = %d, want %d", got, want)
	}

	model.move(1)
	if got, want := len(model.detailMatches), 2; got != want {
		t.Fatalf("after session change len(detailMatches) = %d, want %d", got, want)
	}
	if got, want := model.activeDetailMatch, 0; got != want {
		t.Fatalf("activeDetailMatch after session change = %d, want %d", got, want)
	}

	model.details.Width = 12
	model.syncDetails(false)
	narrowLines := len(model.detailLines)
	model.details.Width = 40
	model.syncDetails(false)
	wideLines := len(model.detailLines)
	if narrowLines <= wideLines {
		t.Fatalf("expected narrower width to produce more wrapped lines: narrow=%d wide=%d", narrowLines, wideLines)
	}
	if got, want := len(model.detailMatches), 2; got != want {
		t.Fatalf("after resize len(detailMatches) = %d, want %d", got, want)
	}
}

func TestDetailSearchZeroMatchesDoesNotMoveViewport(t *testing.T) {
	model := testModelWithSessions([]claude.Session{
		testSession("session-1", []claude.Entry{
			{Kind: claude.EntryAssistantText, Content: "alpha"},
			{Kind: claude.EntryAssistantText, Content: "beta"},
			{Kind: claude.EntryAssistantText, Content: "gamma"},
			{Kind: claude.EntryAssistantText, Content: "delta"},
		}),
	})

	model.focus = focusDetails
	model.details.SetYOffset(2)
	model.detailSearch.SetValue("zzz")
	model.detailSearchActive = true
	model.recomputeDetailMatches(false)

	if got := len(model.detailMatches); got != 0 {
		t.Fatalf("len(detailMatches) = %d, want 0", got)
	}
	if got, want := model.details.YOffset, 2; got != want {
		t.Fatalf("details.YOffset = %d, want %d", got, want)
	}
	if got, want := model.renderDetailFooter(), "/zzz                              0 of 0"; got != want {
		t.Fatalf("renderDetailFooter() = %q, want %q", got, want)
	}
}

func testModelWithSessions(sessions []claude.Session) Model {
	search := textinput.New()
	detailSearch := newDetailSearchInput()

	model := Model{
		search:       search,
		detailSearch: detailSearch,
		sessions:     sessions,
		filtered:     sessions,
	}
	model.list = viewport.New(32, 8)
	model.details = viewport.New(40, 8)
	model.syncList()
	model.syncDetails(true)
	return model
}

func testSession(id string, transcript []claude.Entry) claude.Session {
	return claude.Session{
		ID:            id,
		Summary:       id,
		Path:          "/tmp/" + id + ".jsonl",
		UpdatedAt:     time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		StartedAt:     time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		MessageCount:  len(transcript),
		AssistantMsgs: len(transcript),
		Transcript:    transcript,
	}
}

func pressRunes(t *testing.T, model Model, runes ...rune) Model {
	t.Helper()
	for _, r := range runes {
		model = pressKeyMsg(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return model
}

func pressKey(t *testing.T, model Model, key tea.KeyType) Model {
	t.Helper()
	return pressKeyMsg(t, model, tea.KeyMsg{Type: key})
}

func pressKeyMsg(t *testing.T, model Model, msg tea.KeyMsg) Model {
	t.Helper()
	updated, _ := model.Update(msg)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", updated)
	}
	return next
}

func expectedScrollOffset(model Model, matchIndex int) int {
	line := model.detailMatches[matchIndex].line
	maxOffset := max(0, len(model.detailLines)-model.details.Height)
	if line > maxOffset {
		return maxOffset
	}
	return line
}
