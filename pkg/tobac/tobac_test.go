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
	"serviceuser-%s",
}

var emptyResource = &tobac.KubernetesResource{

}

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
	assert.NoError(t, tobac.Allowed(
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
	))
}

func TestRequireTeamLabel(t *testing.T) {
	err := tobac.Allowed(
		tobac.Request{
			UserInfo:             authenticationv1.UserInfo{},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         emptyTeamProvider,
			SubmittedResource:    emptyResource,
		},
	)
	assert.EqualError(t, err, tobac.ErrorNotTaggedWithTeamLabel)
}

func TestRequireTeamExists(t *testing.T) {
	err := tobac.Allowed(
		tobac.Request{
			UserInfo:             authenticationv1.UserInfo{},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         emptyTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
		},
	)
	assert.EqualError(t, err, fmt.Sprintf(tobac.ErrorTeamDoesNotExistInAzureAD, "foo"))
}

func TestRequireExistingTeamExists(t *testing.T) {
	err := tobac.Allowed(
		tobac.Request{
			UserInfo:             authenticationv1.UserInfo{},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("does-not-exist"),
		},
	)
	assert.EqualError(t, err, fmt.Sprintf(tobac.ErrorExistingTeamDoesNotExistInAzureAD, "does-not-exist"))
}

func TestRequireUserInExistingTeam(t *testing.T) {
	err := tobac.Allowed(
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
	assert.EqualError(t, err, fmt.Sprintf(tobac.ErrorUserHasNoAccessToTeam, "bar", "foo"))
}

func TestAllowIfUserExistsInTeamCreate(t *testing.T) {
	err := tobac.Allowed(
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
	assert.NoError(t, err)
}

func TestAllowIfUserExistsInTeamUpdate(t *testing.T) {
	err := tobac.Allowed(
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
	assert.NoError(t, err)
}

func TestAllowServiceUserCreate(t *testing.T) {
	err := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "serviceuser-foo",
				Groups:   []string{},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
		},
	)
	assert.NoError(t, err)
}

func TestAllowServiceUserUpdate(t *testing.T) {
	err := tobac.Allowed(
		tobac.Request{
			UserInfo: authenticationv1.UserInfo{
				Username: "serviceuser-foo",
				Groups:   []string{},
			},
			ClusterAdmins:        clusterAdmins,
			ServiceUserTemplates: serviceUserTemplates,
			TeamProvider:         mockedTeamProvider,
			SubmittedResource:    resourceWithTeam("foo"),
			ExistingResource:     resourceWithTeam("foo"),
		},
	)
	assert.NoError(t, err)
}

func TestAnnexationOfUnlabeledResource(t *testing.T) {
	err := tobac.Allowed(
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
	assert.NoError(t, err)
}

func TestAnnexationOfLabeledResource(t *testing.T) {
	err := tobac.Allowed(
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
	assert.EqualError(t, err, fmt.Sprintf(tobac.ErrorUserHasNoAccessToTeam, "bar", "baz"))
}

func TestMoveResourceToNewTeam(t *testing.T) {
	err := tobac.Allowed(
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
	assert.NoError(t, err)
}
