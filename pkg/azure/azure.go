package azure

import (
	"context"
	"encoding/json"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/oauth2/microsoft"
	"k8s.io/klog/glog"
	"net/http"
	"net/url"
	"os"
	"time"
)

var (
	clientId     = os.Getenv("AZURE_APP_ID")
	clientSecret = os.Getenv("AZURE_PASSWORD")
	tenantId     = os.Getenv("AZURE_TENANT")
)

const SHAREPOINT_QUERY = "9f0d0ea1-0226-4aa9-9bf9-b6e75816fabf/sites/root/lists/nytt team/items?expand=fields"

type sharePointList struct {
	Value []sharePointListEntry `json:"value"`
}

type sharePointListEntry struct {
	Fields Team `json:"fields"`
}

type Team struct {
	AzureUUID   string `json:"GruppeID"`
	ID          string `json:"mailnick_x002f_tag"`
	Title       string `json:"Title"`
	Description string `json:"Beskrivelse"`
}

// Valid returns true if the ID fields are non-empty.
func (team Team) Valid() bool {
	return len(team.AzureUUID) > 0 && len(team.ID) > 0
}

var cachedClient *http.Client

func client(ctx context.Context) *http.Client {
	if cachedClient != nil {
		return cachedClient
	}

	config := clientcredentials.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Scopes:       []string{"https://graph.microsoft.com/.default"},
		TokenURL:     microsoft.AzureADEndpoint(tenantId).TokenURL,
	}

	cachedClient = config.Client(ctx)
	return cachedClient
}

func get(ctx context.Context, path string, target interface{}) error {
	getUrl, err := url.Parse("https://graph.microsoft.com/v1.0/groups/" + path)
	if err != nil {
		return err
	}

	req := &http.Request{
		Method: "GET",
		URL:    getUrl,
	}

	resp, err := client(ctx).Do(req)
	if err != nil {
		return err
	}

	json.NewDecoder(resp.Body).Decode(target)

	return nil
}

// Teams retrieves the canonical list of team groups from the Microsoft Graph API.
func Teams(ctx context.Context) (map[string]Team, error) {
	teams := make(map[string]Team)

	list := &sharePointList{}
	err := get(ctx, SHAREPOINT_QUERY, list)
	if err != nil {
		return nil, err
	}

	for _, v := range list.Value {
		team := v.Fields
		if team.Valid() {
			teams[team.ID] = team
			glog.V(9).Infof("azure: add team '%s' with id '%s'", team.ID, team.AzureUUID)
		}
	}

	return teams, nil
}

// DefaultContext returns a context that will time out after one second.
// Remember to call CancelFunc when you are done.
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 1*time.Second)
}
