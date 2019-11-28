package azure

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

type GraphAPI struct {
	client *http.Client
}

type ServicePrincipal struct {
	PrincipalID   string `json:"principalId"`
	PrincipalType string `json:"principalType"`
}

type ServicePrincipalList struct {
	NextLink string             `json:"@odata.nextLink"`
	Value    []ServicePrincipal `json:"value"`
}

type Group struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	MailNickname string `json:"mailNickname"`
}

type GroupList struct {
	Value []Group
}

func NewGraphAPI(client *http.Client) *GraphAPI {
	return &GraphAPI{
		client: client,
	}
}

// Retrieve a list of Azure Groups that are given access to a specific Azure Application.
func (g *GraphAPI) GroupsFromApplication(appID string) ([]Group, error) {
	servicePrincipals, err := g.servicePrincipalsInApplication(appID)
	if err != nil {
		return nil, fmt.Errorf("get parent group: %s", err)
	}

	groups := make([]Group, 0)
	for _, servicePrincipal := range servicePrincipals {
		if servicePrincipal.PrincipalType != "Group" {
			continue
		}
		group, err := g.group(servicePrincipal.PrincipalID)
		if err != nil {
			return nil, fmt.Errorf("recurse into groups: %s", err)
		}
		groups = append(groups, *group)
	}

	return groups, nil
}

// https://docs.microsoft.com/en-us/graph/api/approleassignment-get?view=graph-rest-beta&tabs=http
func (g *GraphAPI) servicePrincipalsInApplication(appID string) ([]ServicePrincipal, error) {
	servicePrincipals := make([]ServicePrincipal, 0)

	queryParams := url.Values{}
	queryParams.Set("$top", "999")
	queryParams.Set("$select", "principalId,principalType")
	nextURL := fmt.Sprintf("https://graph.microsoft.com/beta/servicePrincipals/%s/appRoleAssignments?%s", appID, queryParams.Encode())

	for len(nextURL) != 0 {
		_, body, err := g.query(nextURL)
		if err != nil {
			return nil, err
		}

		servicePrincipalList := &ServicePrincipalList{}
		err = json.Unmarshal(body, servicePrincipalList)
		if err != nil {
			return nil, err
		}
		servicePrincipals = append(servicePrincipals, servicePrincipalList.Value...)
		nextURL = servicePrincipalList.NextLink
	}

	return servicePrincipals, nil
}

func (g *GraphAPI) group(groupID string) (*Group, error) {
	u := fmt.Sprintf("https://graph.microsoft.com/v1.0/groups/%s", groupID)

	queryParams := url.Values{}
	queryParams.Set("$select", "id,displayName,mailNickname")

	group := &Group{}
	_, body, err := g.query(u + "?" + queryParams.Encode())
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, group)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func (g *GraphAPI) query(url string) (response *http.Response, body []byte, err error) {
	response, err = g.client.Get(url)
	if err != nil {
		return
	}

	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	if response.StatusCode > 299 {
		err = fmt.Errorf("%s: %s", response.Status, string(body))
	}

	return
}
