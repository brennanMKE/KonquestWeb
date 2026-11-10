package room

import (
	"os"
	"path/filepath"
	"testing"

	"parallax/internal/proto"
)

func TestRoomsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	var n uint64
	opts := Options{Logger: quietLogger(), NewSeed: func() uint64 { n++; return n }}
	m := NewManager(opts)
	r := m.Create()
	code := r.Code()

	ann := connect(t, r)
	ann.send(&proto.Join{Name: "Ann"})
	token := expect[proto.Welcome](ann).Token
	ben := connect(t, r)
	ben.send(&proto.Join{Name: "Ben"})
	benToken := expect[proto.Welcome](ben).Token
	s := proto.DefaultSettings()
	s.AIDelayMs = 0
	ann.send(&proto.SetSettings{Settings: s})
	ann.send(&proto.AddAI{})
	ann.send(&proto.StartGame{})
	yt := expect[proto.YourTurn](ann)
	var home int
	for _, p := range yt.View.Planets {
		if p.Owner == 0 {
			home = p.ID
		}
	}
	ann.send(&proto.SendFleet{From: home, To: (home + 1) % len(yt.View.Planets), Ships: 3})
	expect[proto.FleetAck](ann)
	ann.send(&proto.EndTurn{})
	expect[proto.YourTurn](ben)
	// A lobby room comes back too.
	lobby := m.Create()

	if saved, err := m.SaveAll(dir); err != nil || saved != 2 {
		t.Fatalf("SaveAll = %d, %v", saved, err)
	}
	info, err := os.Stat(filepath.Join(dir, code+".json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("room file: %v %v", info, err)
	}
	r.Close()
	lobby.Close()
	<-r.Done()
	<-lobby.Done()

	m2 := NewManager(opts)
	if got := m2.RestoreAll(dir); got != 2 {
		t.Fatalf("RestoreAll = %d", got)
	}
	r2, ok := m2.Get(code)
	if !ok {
		t.Fatal("room not reopened")
	}
	t.Cleanup(r2.Close)
	if _, ok := m2.Get(lobby.Code()); !ok {
		t.Fatal("lobby room not reopened")
	}

	// Ben rejoins with his token, mid-turn, where he was.
	ben2 := connect(t, r2)
	ben2.send(&proto.Join{Token: benToken})
	if w := expect[proto.Welcome](ben2); w.Seat != 1 || w.Host {
		t.Fatalf("ben: %+v", w)
	}
	bt := expect[proto.YourTurn](ben2)
	if bt.Round != 1 || len(bt.View.Fleets) != 0 {
		t.Fatalf("ben's turn: round %d, %d fleets", bt.Round, len(bt.View.Fleets))
	}
	ann2 := connect(t, r2)
	ann2.send(&proto.Join{Token: token})
	if w := expect[proto.Welcome](ann2); w.Seat != 0 || !w.Host {
		t.Fatalf("ann: %+v", w)
	}
	pt := expect[proto.PlayerTurn](ann2)
	if len(pt.View.Fleets) != 1 || pt.View.Fleets[0].Ships != 3 {
		t.Fatalf("ann's fleets after restart: %+v", pt.View.Fleets)
	}
	ben2.send(&proto.EndTurn{})
	expect[proto.RoundResult](ann2)

	// Closed rooms' files go on the next save.
	m2.Get(lobby.Code())
	l2, _ := m2.Get(lobby.Code())
	l2.Close()
	<-l2.Done()
	if saved, err := m2.SaveAll(dir); err != nil || saved != 1 {
		t.Fatalf("second SaveAll = %d, %v", saved, err)
	}
	if _, err := os.Stat(filepath.Join(dir, lobby.Code()+".json")); !os.IsNotExist(err) {
		t.Errorf("closed room's file is still there: %v", err)
	}

	// A broken file is skipped.
	os.WriteFile(filepath.Join(dir, "ZZZZ.json"), []byte(`{"version": 1, "code": "ZZZZ", "phase": "nope"}`), 0o600)
	if got := NewManager(opts).RestoreAll(dir); got != 1 {
		t.Errorf("RestoreAll with a bad file = %d, want 1", got)
	}
}
