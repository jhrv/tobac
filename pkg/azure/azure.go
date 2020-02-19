package azure

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/oauth2/microsoft"
)

var (
	clientID                    = os.Getenv("AZURE_APP_ID")
	clientSecret                = os.Getenv("AZURE_PASSWORD")
	tenantID                    = os.Getenv("AZURE_TENANT")
	teamMembershipApplicationID = os.Getenv("AZURE_TEAM_MEMBERSHIP_APP_ID")
)

type Team struct {
	AzureUUID   string
	ID          string
	Title       string
	Description string
}

// Valid returns true if the ID fields are non-empty.
func (team Team) Valid() bool {
	return len(team.AzureUUID) > 0 && len(team.ID) > 0
}

func client(ctx context.Context) *http.Client {
	config := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"https://graph.microsoft.com/.default"},
		TokenURL:     microsoft.AzureADEndpoint(tenantID).TokenURL,
	}

	return config.Client(ctx)
}

// Teams retrieves the canonical list of team groups from the Microsoft Graph API.
func Teams(ctx context.Context) (map[string]Team, error) {
	graphAPI := NewGraphAPI(client(ctx))

	teamGroups, err := graphAPI.GroupsFromApplication(teamMembershipApplicationID)
	if err != nil {
		return nil, err
	}

	teams := make(map[string]Team)
	for _, teamGroup := range teamGroups {
		team := Team{
			AzureUUID: teamGroup.ID,
			Title:     teamGroup.DisplayName,
			ID:        strings.ToLower(teamGroup.MailNickname),
		}
		if team.Valid() {
			teams[team.ID] = team
			log.Debugf("azure: add team '%s' with id '%s'", team.ID, team.AzureUUID)
		} else {
			log.Errorf("azure: invalid team '%s'", team.ID)
		}
		if team.ID != teamGroup.MailNickname {
			log.Warnf("azure: transposing real team name '%s' to lowercase '%s'", teamGroup.MailNickname, team.ID)
		}
	}

	return teams, nil
}

// DefaultContext returns a context that will time out.
// Remember to call CancelFunc when you are done.
func DefaultContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
