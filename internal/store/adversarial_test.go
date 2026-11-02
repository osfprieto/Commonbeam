package store_test

import (
	"testing"

	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/testutil"
)

func TestSignedSubstitutionIsObservedWithoutClaimingProof(t *testing.T) {
	db, _, _ := testutil.Database(t)
	m, key, _ := enroll(t, db, 1)
	token := grant(t, db)
	j, err := db.Reserve(ctx, token, protocol.RandomID(), request())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Claim(ctx, m.Provider); err != nil {
		t.Fatal(err)
	}
	output := "substituted work"
	if err := db.Append(ctx, m.Provider, j.ID, output); err != nil {
		t.Fatal(err)
	}
	input := request().InputBudget()
	r := protocol.SignReceipt(protocol.Receipt{Version: protocol.Version, Network: db.Network,
		Job: j.ID, RequestHash: j.Hash, Provider: m.Provider, Model: m.Model, Profile: m.Profile,
		OutputHash: protocol.OutputCommitment(j.Salt, output), Simulation: true,
		Usage: protocol.Usage{PromptTokens: input, CompletionTokens: 2, TotalTokens: input + 2}}, key)
	if err := db.Finish(ctx, m.Provider, j.ID, &r); err != nil {
		t.Fatal(err)
	}
	report, err := db.Audit(ctx, j.ID)
	if err != nil || report["receipt_valid"] != true || report["simulation_replay_match"] != false ||
		report["financial_penalty"] != false || report["execution_proven"] != false {
		t.Fatalf("signature was confused with execution proof: %v %v", report, err)
	}
}
