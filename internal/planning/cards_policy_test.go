package planning

import "testing"

func TestDecideCards(t *testing.T) {
	policy := CardsPolicy{
		RequiredWhen: []string{
			"parallel_teams>=2",
			"must_answer_count>=6",
			"conflict_detected=true",
		},
		OptionalWhen: []string{
			"parallel_teams==1 && must_answer_count<=5 && has_tradeoffs=false",
		},
	}

	t.Run("required by parallel teams", func(t *testing.T) {
		dec, err := DecideCards(policy, CardsDecisionInput{ParallelTeams: 2})
		if err != nil {
			t.Fatalf("DecideCards: %v", err)
		}
		if !dec.Required {
			t.Fatalf("expected required=true")
		}
		if len(dec.RequiredReasons) == 0 {
			t.Fatalf("expected reasons")
		}
	})

	t.Run("not required, optional matched", func(t *testing.T) {
		dec, err := DecideCards(policy, CardsDecisionInput{
			ParallelTeams:   1,
			MustAnswerCount: 5,
			HasTradeoffs:    false,
		})
		if err != nil {
			t.Fatalf("DecideCards: %v", err)
		}
		if dec.Required {
			t.Fatalf("expected required=false")
		}
		if len(dec.OptionalReasons) != 1 {
			t.Fatalf("expected optionalReasons=1, got %d", len(dec.OptionalReasons))
		}
	})

	t.Run("invalid rule", func(t *testing.T) {
		_, err := DecideCards(CardsPolicy{
			RequiredWhen: []string{"parallel_teams>="},
		}, CardsDecisionInput{ParallelTeams: 2})
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}
