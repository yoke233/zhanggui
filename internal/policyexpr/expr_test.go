package policyexpr

import "testing"

func TestParseBoolExpr_Eval_BasicComparisons(t *testing.T) {
	expr, err := ParseBoolExpr("parallel_teams>=2")
	if err != nil {
		t.Fatalf("ParseBoolExpr: %v", err)
	}

	ok, err := expr.Eval(map[string]any{"parallel_teams": 2})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !ok {
		t.Fatalf("expected true")
	}

	ok, err = expr.Eval(map[string]any{"parallel_teams": 1})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if ok {
		t.Fatalf("expected false")
	}
}

func TestParseBoolExpr_Eval_AndOrAndParens(t *testing.T) {
	expr, err := ParseBoolExpr("(parallel_teams==1 && must_answer_count<=5 && has_tradeoffs=false) || conflict_detected=true")
	if err != nil {
		t.Fatalf("ParseBoolExpr: %v", err)
	}

	{
		ok, err := expr.Eval(map[string]any{
			"parallel_teams":     1,
			"must_answer_count":  5,
			"has_tradeoffs":      false,
			"conflict_detected":  false,
			"evidence_required":  false,
			"needs_comparison":   false,
			"needs_comparison_2": false,
		})
		if err != nil {
			t.Fatalf("Eval: %v", err)
		}
		if !ok {
			t.Fatalf("expected true")
		}
	}

	{
		ok, err := expr.Eval(map[string]any{
			"parallel_teams":    3,
			"must_answer_count": 10,
			"has_tradeoffs":     true,
			"conflict_detected": true,
		})
		if err != nil {
			t.Fatalf("Eval: %v", err)
		}
		if !ok {
			t.Fatalf("expected true")
		}
	}

	{
		ok, err := expr.Eval(map[string]any{
			"parallel_teams":    3,
			"must_answer_count": 10,
			"has_tradeoffs":     true,
			"conflict_detected": false,
		})
		if err != nil {
			t.Fatalf("Eval: %v", err)
		}
		if ok {
			t.Fatalf("expected false")
		}
	}
}

func TestParseBoolExpr_Eval_SingleEqualsAndStrings(t *testing.T) {
	expr, err := ParseBoolExpr(`where=verify && severity!="info"`)
	if err != nil {
		t.Fatalf("ParseBoolExpr: %v", err)
	}
	ok, err := expr.Eval(map[string]any{
		"where":    "verify",
		"severity": "warn",
	})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !ok {
		t.Fatalf("expected true")
	}
}

func TestParseBoolExpr_Errors(t *testing.T) {
	t.Run("missing var", func(t *testing.T) {
		expr, err := ParseBoolExpr("must_answer_count>=6")
		if err != nil {
			t.Fatalf("ParseBoolExpr: %v", err)
		}
		_, err = expr.Eval(map[string]any{})
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("bad syntax", func(t *testing.T) {
		_, err := ParseBoolExpr("parallel_teams>=")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("type mismatch", func(t *testing.T) {
		expr, err := ParseBoolExpr("parallel_teams>=2")
		if err != nil {
			t.Fatalf("ParseBoolExpr: %v", err)
		}
		_, err = expr.Eval(map[string]any{"parallel_teams": "two"})
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}
