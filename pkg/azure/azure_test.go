package azure_test

import (
	"fmt"
	"github.com/nais/tobac/pkg/azure"
	"testing"
)

func TestAzure(t *testing.T) {
	ctx, _ := azure.DefaultContext()

	teams, err := azure.Teams(ctx)
	if err != nil {
		fmt.Printf("%s", err)
	}

	for _, team := range teams {
		fmt.Printf("%+v\n", team)
	}
}
