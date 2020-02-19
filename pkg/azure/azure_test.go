// +build integration

package azure_test

import (
	"testing"
	"time"

	"github.com/nais/tobac/pkg/azure"
)

func TestAzure(t *testing.T) {
	ctx, _ := azure.DefaultContext(1 * time.Second)

	t.Logf("Running live integration test to get teams from Azure AD")

	teams, err := azure.Teams(ctx)
	if err != nil {
		t.Errorf("Azure AD returned error: %s", err)
		t.FailNow()
	}

	t.Logf("Azure AD returned successfully with %d teams", len(teams))

	n := 0
	for _, team := range teams {
		n++
		t.Logf("[%03d]: %+v\n", n, team)
	}
}
