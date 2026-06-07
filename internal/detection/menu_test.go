package detection

import "testing"

func TestFindStopAndWaitMove_NewTwoOptionFormat(t *testing.T) {
	content := `   What do you want to do?

   ❯ 1. Upgrade your plan
     2. Stop and wait for limit to reset

   Enter to confirm · Esc to cancel
`
	move, ok := FindStopAndWaitMove(content)
	if !ok {
		t.Fatal("expected to find 'Stop and wait' row")
	}
	if move != 1 {
		t.Errorf("expected move=+1 (one Down), got %d", move)
	}
}

func TestFindStopAndWaitMove_OldThreeOptionFormat(t *testing.T) {
	content := `│ What do you want to do?                          │
│                                                  │
│ ❯ 1. Stop and wait for limit to reset            │
│   2. Switch to extra usage                       │
│   3. Upgrade your plan                           │
`
	move, ok := FindStopAndWaitMove(content)
	if !ok {
		t.Fatal("expected to find 'Stop and wait' row")
	}
	if move != 0 {
		t.Errorf("expected move=0 (cursor already on row), got %d", move)
	}
}

func TestFindStopAndWaitMove_CursorOnSecond(t *testing.T) {
	content := `   1. Upgrade your plan
   ❯ 2. Switch to extra usage
     3. Stop and wait for limit to reset
`
	move, ok := FindStopAndWaitMove(content)
	if !ok {
		t.Fatal("expected to find 'Stop and wait' row")
	}
	if move != 1 {
		t.Errorf("expected move=+1 (Down once), got %d", move)
	}
}

func TestFindStopAndWaitMove_NoMenu(t *testing.T) {
	content := "Just some normal chat text\n> prompt\n"
	if _, ok := FindStopAndWaitMove(content); ok {
		t.Error("expected ok=false when no menu present")
	}
}

func TestFindStopAndWaitMove_NoStopAndWaitRow(t *testing.T) {
	content := `  ❯ 1. Option A
    2. Option B
`
	if _, ok := FindStopAndWaitMove(content); ok {
		t.Error("expected ok=false when 'Stop and wait' missing")
	}
}
