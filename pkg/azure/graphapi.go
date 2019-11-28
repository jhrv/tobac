package azure

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// see https://docs.microsoft.com/en-us/graph/extensibility-schema-groups

// List members
// https://docs.microsoft.com/en-us/graph/api/group-list-members?view=graph-rest-1.0&tabs=http

// With Azure AD resources that derive from directoryObject, like user and group,
// $expand is only supported for beta and typically returns a maximum of 20 items for the expanded relationship.

type GraphAPI struct {
	client *http.Client
}

type DirectoryObject struct {
	ID string `json:"id"`
}

type DirectoryObjectList struct {
	Value []DirectoryObject `json:"value"`
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

func (g *GraphAPI) GroupsInGroup(groupID string) ([]Group, error) {
	directoryObjects, err := g.directoryObjectsInGroup(groupID)
	if err != nil {
		return nil, fmt.Errorf("get parent group: %s", err)
	}

	groups := make([]Group, len(directoryObjects))
	for i, directoryObject := range directoryObjects {
		group, err := g.group(directoryObject.ID)
		if err != nil {
			return nil, fmt.Errorf("recurse into groups: %s", err)
		}
		groups[i] = *group
	}

	return groups, nil
}

func (g *GraphAPI) directoryObjectsInGroup(groupID string) ([]DirectoryObject, error) {
	u := fmt.Sprintf("https://graph.microsoft.com/v1.0/groups/%s/members", groupID)
	_, body, err := g.query(u, url.Values{})
	if err != nil {
		return nil, err
	}

	directoryObjectList := &DirectoryObjectList{}
	err = json.Unmarshal(body, directoryObjectList)
	if err != nil {
		return nil, err
	}

	return directoryObjectList.Value, nil
}
func (g *GraphAPI) group(groupID string) (*Group, error) {
	u := fmt.Sprintf("https://graph.microsoft.com/v1.0/groups/%s", groupID)

	queryParams := url.Values{}
	queryParams.Set("$select", "id,displayName,mailNickname")

	group := &Group{}
	_, body, err := g.query(u, queryParams)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, group)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func (g *GraphAPI) query(url string, queryParams url.Values) (response *http.Response, body []byte, err error) {
	urlWithParams := url + "?" + queryParams.Encode()

	response, err = g.client.Get(urlWithParams)
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
