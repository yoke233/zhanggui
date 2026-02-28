package core

import "testing"

func TestHumanActionTypeValidate(t *testing.T) {
	actions := []HumanActionType{
		ActionApprove,
		ActionReject,
		ActionModify,
		ActionSkip,
		ActionRerun,
		ActionChangeAgent,
		ActionAbort,
		ActionPause,
		ActionResume,
	}

	for _, action := range actions {
		if err := action.Validate(); err != nil {
			t.Fatalf("expected valid action %s, got %v", action, err)
		}
	}
}
