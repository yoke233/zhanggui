package core

import "testing"

func TestActionTypeCompositeExists(t *testing.T) {
	if ActionComposite != "composite" {
		t.Fatalf("ActionComposite = %q, want %q", ActionComposite, "composite")
	}
}

func TestActionTypeValid(t *testing.T) {
	valid := []ActionType{ActionExec, ActionGate, ActionPlan, ActionComposite}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("ActionType(%q).Valid() = false, want true", v)
		}
	}
	if ActionType("bogus").Valid() {
		t.Error("ActionType(bogus).Valid() = true, want false")
	}
}

func TestParseActionType(t *testing.T) {
	got, err := ParseActionType("exec")
	if err != nil || got != ActionExec {
		t.Fatalf("ParseActionType(exec) = (%q, %v), want (exec, nil)", got, err)
	}
	_, err = ParseActionType("invalid")
	if err == nil {
		t.Fatal("ParseActionType(invalid) should return error")
	}
}

func TestActionStatusValid(t *testing.T) {
	valid := []ActionStatus{ActionPending, ActionReady, ActionRunning, ActionWaitingGate, ActionBlocked, ActionFailed, ActionDone, ActionCancelled}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("ActionStatus(%q).Valid() = false, want true", v)
		}
	}
	if ActionStatus("bogus").Valid() {
		t.Error("ActionStatus(bogus).Valid() = true, want false")
	}
}

func TestParseActionStatus(t *testing.T) {
	got, err := ParseActionStatus("running")
	if err != nil || got != ActionRunning {
		t.Fatalf("ParseActionStatus(running) = (%q, %v), want (running, nil)", got, err)
	}
	_, err = ParseActionStatus("nope")
	if err == nil {
		t.Fatal("ParseActionStatus(nope) should return error")
	}
}
