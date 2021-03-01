package tobac_test

import (
	"fmt"
	"testing"

	"github.com/nais/tobac/pkg/azure"
	"github.com/nais/tobac/pkg/tobac"
	"github.com/stretchr/testify/assert"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var clusterAdmins = []string{
	"cluster-admin",
}

var serviceUserTemplates = []string{
	"system:serviceaccounts:%s:serviceuser-%s",
}

var emptyResource = &tobac.KubernetesResource{}

func resourceWithTeam(team string) *tobac.KubernetesResource {
	return &tobac.KubernetesResource{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"team": team,
			},
		},
	}
}

func emptyTeamProvider(_ string) azure.Team {
	return azure.Team{}
}

func mockedTeamProvider(team string) azure.Team {
	if team == "does-not-exist" {
		return azure.Team{}
	}
	return azure.Team{
		ID:          team,
		Title:       team,
		Description: team,
		AzureUUID:   team,
	}
}

func TestClusterAdmin(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "i-dont-care",
				Groups: []string{
					"cluster-admin",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
		},
	)
	assert.True(t, response.Allowed)
}

func TestRequireTeamLabel(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo:             authenticationv1.UserInfo{},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         emptyTeamProvider,
			SubmittedResource:    emptyResource,
		},
	)
	assert.False(t, response.Allowed)
	assert.Equal(t, tobac.ErrorNotTaggedWithTeamLabel, response.Reason)
}

func TestRequireTeamExists(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo:             authenticationv1.UserInfo{},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         emptyTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
		},
	)
	assert.False(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.ErrorTeamDoesNotExistInAzureAD, "foo"), response.Reason)
}

func TestRequireExistingTeamExists(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo:             authenticationv1.UserInfo{},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("does-not-exist"),
		},
	)
	assert.False(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.ErrorExistingTeamDoesNotExistInAzureAD, "does-not-exist"), response.Reason)
}

func TestRequireUserInExistingTeam(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups:   []string{},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("foo"),
		},
	)
	assert.False(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.ErrorUserHasNoAccessToTeam, "bar", "foo"), response.Reason)
}

func TestAllowIfUserExistsInTeamCreate(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups: []string{
					"foo",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.SuccessUserBelongsToTeam, "foo"), response.Reason)
}

func TestAllowIfUserExistsInTeamUpdate(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups: []string{
					"foo",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("foo"),
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.SuccessUserBelongsToTeam, "foo"), response.Reason)
}

func TestAllowIfUserExistsInTeamDelete(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups: []string{
					"foo",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			ExistingResource:     resourceWithTeam("foo"),
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.SuccessUserBelongsToTeam, "foo"), response.Reason)
}
func TestAllowServiceUserCreate(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccounts:foo:serviceuser-foo",
				Groups:   []string{},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.SuccessUserMatchesServiceUserTemplate), response.Reason)
}

func TestAllowServiceUserUpdate(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccounts:foo:serviceuser-foo",
				Groups:   []string{},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("foo"),
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.SuccessUserMatchesServiceUserTemplate), response.Reason)
}

func TestAllowServiceUserDelete(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccounts:foo:serviceuser-foo",
				Groups:   []string{},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			ExistingResource:     resourceWithTeam("foo"),
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.SuccessUserMatchesServiceUserTemplate), response.Reason)
}

func TestAnnexationOfUnlabeledResource(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups: []string{
					"foo",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     emptyResource,
		},
	)
	assert.True(t, response.Allowed)
	assert.Equal(t, tobac.SuccessUserMayAnnexateOrphanResource, response.Reason)
}

func TestAnnexationOfLabeledResource(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups: []string{
					"foo",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("baz"),
		},
	)
	assert.False(t, response.Allowed)
	assert.Equal(t, fmt.Sprintf(tobac.ErrorUserHasNoAccessToTeam, "bar", "baz"), response.Reason)
}

func TestMoveResourceToNewTeam(t *testing.T) {
	response := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "bar",
				Groups: []string{
					"old-team",
					"new-team",
				},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("new-team"),
			ExistingResource:     resourceWithTeam("old-team"),
		},
	)
	assert.True(t, response.Allowed)
}
