package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/oauth2/microsoft"
	"net/http"
	"net/url"
	"os"
	"testing"
)

var (
	clientId     = os.Getenv("AZURE_AD_SERVICE_PRINCIPAL_APP_ID")
	clientSecret = os.Getenv("AZURE_AD_SERVICE_PRINCIPAL_PASSWORD")
	tenantId     = os.Getenv("AZURE_AD_SERVICE_PRINCIPAL_TENANT")
)

type SharePointList struct {
	Value []ListGroupEntry `json:"value"`
}

type ListGroupEntry struct {
	Fields ListGroupEntryFields `json:"fields"`
}

type ListGroupEntryFields struct {
	GroupID string `json:"GruppeID"`
}

type Group struct {
	Id   string `json:"id"`
	Mail string `json:"mail"`
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

func getGroupIds(ctx context.Context) ([]string, error) {
	list := &SharePointList{}
	err := get(ctx, "9f0d0ea1-0226-4aa9-9bf9-b6e75816fabf/sites/root/lists/nytt team/items?expand=fields", list)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0)
	for _, v := range list.Value {
		if len(v.Fields.GroupID) > 0 {
			ids = append(ids, v.Fields.GroupID)
		}
	}

	return ids, nil
}

func getGroup(ctx context.Context, groupId string) (*Group, error) {
	group := &Group{}
	err := get(ctx, groupId, group)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func TestAzure(t *testing.T) {
	ctx := context.Background()

	ids, err := getGroupIds(ctx)
	if err != nil {
		fmt.Printf("%s", err)
	}

	for _, groupId := range ids {
		group, err := getGroup(ctx, groupId)
		if err != nil {
			fmt.Printf("%s", err)
		}
		fmt.Printf("%+v\n", group)
	}
}
