package computer

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/quota"
)

func TestAPersonHasAComputerInEachWorkspaceAndTheFirstKeepsItsName(t *testing.T) {
	r := open(t, "acme")
	first, _, err := r.Ensure("ada@acme.com", "", "cell-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != digest("acme\x00ada@acme.com") || first.Workspace != "" || first.Sandbox != Name("acme", "main", "ada@acme.com") {
		t.Fatalf("the first workspace's computer is %+v, not the name it had before workspaces", first)
	}
	business, made, err := r.Ensure("ada@acme.com", "business", "cell-1")
	if err != nil || !made {
		t.Fatalf("a second workspace made no computer of its own: made %v, %v", made, err)
	}
	if business.ID == first.ID || business.Sandbox == first.Sandbox || business.Workspace != "business" || !strings.HasPrefix(business.Sandbox, Prefix("acme")) || len(business.Sandbox) > 19 {
		t.Fatalf("the business computer %+v is not its own, in the organization", business)
	}
	if got, err := r.Of("ada@acme.com", "business"); err != nil || got.ID != business.ID {
		t.Fatalf("asking for the business computer found %+v, %v", got, err)
	}
	if _, err := r.Of("ada@acme.com", "engineering"); err != ErrNone {
		t.Fatalf("a workspace the person has no computer in found one: %v", err)
	}
}

func TestLeavingAWorkspaceRetiresOnlyItsComputerAndLeavingTheOrganizationRetiresAll(t *testing.T) {
	r := open(t, "acme")
	first, _, _ := r.Ensure("ada@acme.com", "", "cell-1")
	business, _, _ := r.Ensure("ada@acme.com", "business", "cell-1")
	bob, _, _ := r.Ensure("bob@acme.com", "business", "cell-1")
	if err := r.RetireIn("ada@acme.com", "business"); err != nil {
		t.Fatal(err)
	}
	if retiring := ids(t, r); retiring != business.ID {
		t.Fatalf("leaving business marked %q, not only %s", retiring, business.ID)
	}
	if err := r.Retire("ada@acme.com"); err != nil {
		t.Fatal(err)
	}
	if retiring := ids(t, r); !strings.Contains(retiring, first.ID) || !strings.Contains(retiring, business.ID) || strings.Contains(retiring, bob.ID) {
		t.Fatalf("leaving the organization marked %q, not both of ada's computers and none of bob's", retiring)
	}
}

func TestEveryWorkspacesComputersCountAgainstTheOrganizationsQuota(t *testing.T) {
	r := open(t, "acme")
	limited(t, r, quota.Quota{Computers: 2})
	r.Ensure("ada@acme.com", "", "cell-1")
	r.Ensure("ada@acme.com", "business", "cell-1")
	if _, _, err := r.Ensure("ada@acme.com", "engineering", "cell-1"); err == nil {
		t.Fatal("a third computer was made past an organization's quota of two, because it was in another workspace")
	}
}

func ids(t *testing.T, r *Registry) string {
	t.Helper()
	retiring, err := r.Retiring()
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, each := range retiring {
		found = append(found, each.ID)
	}
	return strings.Join(found, ",")
}

func marked(r *Registry, id string) bool {
	_, err := os.Stat(r.retireFile(id))
	return err == nil
}

// Where computers are containers a person's home is kept under the id of a computer that has no record, and leaving marks it.
func TestLeavingMarksAHomeThatHasNoRecordAndNotOneThatWasRemoved(t *testing.T) {
	r := open(t, "acme")
	if err := r.RetireIn("Ada@acme.com", "business"); err != nil {
		t.Fatal(err)
	}
	if !marked(r, ID("acme", "business", "ada@acme.com")) {
		t.Error("a home with no record was not marked")
	}
	gone, _, _ := r.Ensure("ada@acme.com", "", "cell-1")
	r.Retire("ada@acme.com")
	if err := r.Retired(gone.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.RetireIn("ada@acme.com", ""); err != nil {
		t.Fatal(err)
	}
	if marked(r, gone.ID) {
		t.Error("a computer that was removed was marked, which would remove the next one made for them")
	}
	if err := r.Retire("ada@acme.com", "", "engineering"); err != nil {
		t.Fatal(err)
	}
	if marked(r, gone.ID) || !marked(r, ID("acme", "engineering", "ada@acme.com")) {
		t.Error("leaving the organization marks the homes of the workspaces the person was in, except a removed computer's")
	}
}
